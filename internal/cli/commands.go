package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/sebihoermann/devdb-go/internal/app"
	"github.com/sebihoermann/devdb-go/internal/domain/analytics"
	"github.com/sebihoermann/devdb-go/internal/domain/feedback"
	"github.com/sebihoermann/devdb-go/internal/domain/planning"
	"github.com/sebihoermann/devdb-go/internal/domain/status"
	"github.com/sebihoermann/devdb-go/internal/importer"
	"github.com/sebihoermann/devdb-go/internal/output"
	"github.com/sebihoermann/devdb-go/internal/storage"
	"github.com/spf13/cobra"
)

// Execute runs the devdb command tree.
func Execute() int {
	root := newRoot()
	if err := root.Execute(); err != nil {
		ce := coerceCLIError(err)
		recordMissedFromError(ce)
		return printCLIError(ce, os.Stderr)
	}
	return ExitOK
}

func coerceCLIError(err error) *CLIError {
	if ce, ok := err.(*CLIError); ok {
		return ce
	}
	msg := err.Error()
	if strings.HasPrefix(msg, "unknown command ") || strings.HasPrefix(msg, "unknown command:") ||
		strings.HasPrefix(msg, "unknown shorthand flag") {
		ce := unknownCommandError(unknownCommandArgv())
		ce.Message = msg
		return ce
	}
	return &CLIError{Code: ExitGeneral, Message: msg, Kind: "cli_error"}
}

func unknownCommandArgv() []string {
	var cmd []string
	for i := 1; i < len(os.Args); i++ {
		a := os.Args[i]
		switch a {
		case "--repo", "--db":
			i++
			continue
		}
		if strings.HasPrefix(a, "-") {
			continue
		}
		cmd = append(cmd, a)
		break
	}
	return cmd
}

func recordMissedFromError(err error) {
	ce, ok := err.(*CLIError)
	if !ok || ce.Kind == "" {
		return
	}
	repo, db := flagRepo, flagDB
	if db == "" {
		db = argvFlag("--db")
	}
	if repo == "" {
		repo = argvFlag("--repo")
	}
	ctx, openErr := app.Open(repo, db, false)
	if openErr != nil {
		return
	}
	defer ctx.Close()
	if err := ctx.RequireDB(); err != nil {
		return
	}
	cwd, _ := os.Getwd()
	analytics.RecordMissedCall(ctx.DB, os.Args[1:], ce.Kind, ce.Message, ce.Suggestion, ce.Code, cwd, ctx.Project.RepoRoot, ctx.ModelID)
}

// validateEnum rejects a flag value that is not in the allowed set,
// returning a CLIError with ExitUsage and a message that names the flag,
// the rejected value, and the allowed values. Used by list-style commands
// whose --status / --severity filters have a closed enum; without this,
// a bogus value silently produces an empty result (feedback [654b7b3e]).
//
// An empty value is treated as "no filter" and passes through. The
// caller decides whether "all" means "no filter" (reminders, tasks)
// or is itself a valid value (feedback), by listing it in `allowed`.
func validateEnum(flagName, value string, allowed []string) error {
	if value == "" {
		return nil
	}
	for _, a := range allowed {
		if value == a {
			return nil
		}
	}
	return usageError(fmt.Sprintf("--%s: invalid value %q (allowed: %s)", flagName, value, strings.Join(allowed, ", ")))
}

func newRoot() *cobra.Command {
	var jsonOut bool

	root := &cobra.Command{
		Use:           "devdb",
		Short:         "Queryable per-project memory for codebases",
		Long:          "Queryable per-project memory for codebases. Use 'devdb help <command> --examples' to see real command shapes and known gotchas for any verb.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVar(&flagRepo, "repo", "", "repository root (default: auto-detect)")
	root.PersistentFlags().StringVar(&flagRepo, "repo-root", "", "alias for --repo (deprecated; for legacy muscle memory)")
	root.PersistentFlags().StringVar(&flagDB, "db", "", "database path (default: .devdb/development.db)")
	root.PersistentFlags().BoolVar(&jsonOut, "json", false, "machine-readable JSON output")
	root.PersistentFlags().BoolVar(&flagAll, "all", false, "expand list limits on read commands")
	root.PersistentFlags().BoolVar(&flagVerbose, "verbose", false, "include diagnostic detail")

	open := func(cmd *cobra.Command) (*app.Context, error) {
		return app.Open(flagRepo, flagDB, jsonOut)
	}

	root.AddCommand(
		cmdInit(open),
		cmdStatus(open),
		cmdQuality(open),
		cmdReport(open),
		cmdResume(open),
		cmdDoctor(open),
		cmdFeedback(open),
		cmdGoal(open),
		cmdFeature(open),
		cmdPlan(open),
		cmdTask(open),
		cmdApproval(open),
		cmdReminder(open),
		cmdArchive(open),
		cmdAnalytics(open),
		cmdLedger(open),
		cmdList(open),
		cmdShow(open),
		cmdInventory(open),
		cmdArch(open),
		cmdReview(open),
		cmdVerify(open),
		cmdImport(open),
		cmdHub(open),
		cmdHelp(),
	)
	return root
}

func cmdInit(open opener) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize .devdb/development.db",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.InitDB(); err != nil {
				return err
			}
			ctx.Out.Hint("initialized %s", ctx.Project.DBPath)
			return ctx.Out.WriteResult(ctx.Project.DBPath, map[string]any{"action": "init"})
		},
	}
}

func cmdStatus(open opener) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Compact delivery snapshot",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			kind, ver, _ := storage.DetectSchema(ctx.DB)
			snap, err := status.Build(ctx.DB, ctx.Project.RepoRoot, string(kind), ver)
			if err != nil {
				return err
			}
			if ctx.Out.JSON {
				if flagVerbose {
					q, _ := status.Quality(ctx.DB)
					return ctx.Out.PrintData(status.VerboseSnapshot{Snapshot: snap, Quality: q})
				}
				return ctx.Out.PrintData(snap)
			}
			if flagVerbose {
				q, _ := status.Quality(ctx.DB)
				lines := []string{
					fmt.Sprintf("overall: %s", snap.Overall),
					fmt.Sprintf("schema: %s v%d", snap.SchemaKind, snap.SchemaVersion),
					fmt.Sprintf("plan items: %d open · %d in progress", snap.OpenItems, snap.InProgress),
					fmt.Sprintf("feedback open: %d", snap.OpenFeedback),
					fmt.Sprintf("tasks open: %d · reminders open: %d", q.OpenTasks, q.OpenReminders),
					fmt.Sprintf("quality: %d high feedback · %d findings · %d missed (7d)",
						q.OpenHighFeedback, q.OpenFindings, q.MissedCalls7d),
				}
				if snap.InFlight != nil {
					lines = append(lines, fmt.Sprintf("in flight: %s (%s)", snap.InFlight.Title, snap.InFlight.ID[:8]))
				}
				if snap.GitBranch != "" {
					dirty := ""
					if snap.GitDirty {
						dirty = " · dirty"
					}
					lines = append(lines, fmt.Sprintf("git: %s%s", snap.GitBranch, dirty))
				}
				return ctx.Out.PrintData(output.HumanLines{Lines: lines})
			}
			lines := []string{fmt.Sprintf("overall: %s", snap.Overall)}
			if snap.InFlight != nil {
				lines = append(lines, fmt.Sprintf("in flight: %s (%s)", snap.InFlight.Title, snap.InFlight.ID[:8]))
			}
			lines = append(lines,
				fmt.Sprintf("plan items: %d open · %d in progress", snap.OpenItems, snap.InProgress),
				fmt.Sprintf("feedback: %d open", snap.OpenFeedback),
			)
			if snap.GitBranch != "" {
				dirty := ""
				if snap.GitDirty {
					dirty = " · dirty"
				}
				lines = append(lines, fmt.Sprintf("git: %s%s", snap.GitBranch, dirty))
			}
			return ctx.Out.PrintData(output.HumanLines{Lines: lines})
		},
	}
}

func cmdQuality(open opener) *cobra.Command {
	return &cobra.Command{
		Use:   "quality",
		Short: "Trust signals snapshot",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			q, err := status.Quality(ctx.DB)
			if err != nil {
				return err
			}
			if ctx.Out.JSON {
				return ctx.Out.PrintData(q)
			}
			lines := []string{
				fmt.Sprintf("open high feedback: %d", q.OpenHighFeedback),
				fmt.Sprintf("stale arch notes: %d", q.StaleArchNotes),
				fmt.Sprintf("open findings: %d", q.OpenFindings),
				fmt.Sprintf("missed calls (7d): %d", q.MissedCalls7d),
			}
			if flagVerbose {
				lines = append(lines,
					fmt.Sprintf("open tasks: %d", q.OpenTasks),
					fmt.Sprintf("open reminders: %d", q.OpenReminders),
				)
			}
			return ctx.Out.PrintData(output.HumanLines{Lines: lines})
		},
	}
}

func cmdReport(open opener) *cobra.Command {
	c := &cobra.Command{
		Use:   "report",
		Short: "Actionable project overview",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			kind, ver, _ := storage.DetectSchema(ctx.DB)
			snap, _ := status.Build(ctx.DB, ctx.Project.RepoRoot, string(kind), ver)
			q, _ := status.Quality(ctx.DB)
			fb, _ := feedback.List(ctx.DB, "open", effectiveListLimit(5))
			if ctx.Out.JSON {
				return ctx.Out.PrintData(map[string]any{
					"status":   snap,
					"quality":  q,
					"feedback": fb,
				})
			}
			lines := []string{
				fmt.Sprintf("# report · %s", snap.Overall),
				"",
				fmt.Sprintf("quality: %d high feedback · %d findings · %d missed (7d)",
					q.OpenHighFeedback, q.OpenFindings, q.MissedCalls7d),
			}
			if len(fb) > 0 {
				lines = append(lines, "", "feedback (open):")
				for _, row := range fb {
					prefix := row.ID[:8]
					sev := row.Severity
					if sev != "" {
						sev = "[" + sev + "] "
					}
					note := row.Note
					if len(note) > 72 {
						note = note[:69] + "..."
					}
					lines = append(lines, fmt.Sprintf("- [%s] %s%s", prefix, sev, note))
				}
			}
			return ctx.Out.PrintData(output.HumanLines{Lines: lines})
		},
	}
	return c
}

func cmdResume(open opener) *cobra.Command {
	return &cobra.Command{
		Use:   "resume",
		Short: "Surface in-flight work",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			item, err := planning.InFlight(ctx.DB)
			if err != nil {
				return err
			}
			if item == nil {
				if ctx.Out.JSON {
					return ctx.Out.PrintData(map[string]any{"in_flight": nil})
				}
				return ctx.Out.PrintData("no in-flight work")
			}
			if ctx.Out.JSON {
				return ctx.Out.PrintData(map[string]any{"in_flight": item})
			}
			return ctx.Out.PrintData(output.HumanLines{Lines: []string{
				fmt.Sprintf("in flight: %s", item.Title),
				fmt.Sprintf("id: %s", item.ID),
				"next: devdb plan item show " + item.ID[:8],
			}})
		},
	}
}

func cmdDoctor(open opener) *cobra.Command {
	doc := &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose database and environment",
	}
	doc.AddCommand(cmdDoctorHygiene(open))
	doc.RunE = func(cmd *cobra.Command, args []string) error {
		ctx, err := open(cmd)
		if err != nil {
			return err
		}
		defer ctx.Close()
		result := map[string]any{
			"repo_root": ctx.Project.RepoRoot,
			"db_path":   ctx.Project.DBPath,
		}
		if _, err := os.Stat(ctx.Project.DBPath); os.IsNotExist(err) {
			result["db_status"] = "missing"
			if ctx.Out.JSON {
				return ctx.Out.PrintData(result)
			}
			return ctx.Out.PrintData("database missing — run devdb init")
		}
		if err := ctx.RequireDB(); err != nil {
			result["db_status"] = "error"
			result["error"] = err.Error()
			if ctx.Out.JSON {
				return ctx.Out.PrintData(result)
			}
			return fmt.Errorf("%s", err.Error())
		}
		kind, ver, _ := storage.DetectSchema(ctx.DB)
		result["schema_kind"] = kind
		result["schema_version"] = ver
		result["db_status"] = "ok"
		if ctx.Out.JSON {
			return ctx.Out.PrintData(result)
		}
		return ctx.Out.PrintData(output.HumanLines{Lines: []string{
			"doctor: ok",
			fmt.Sprintf("schema: %s v%d", kind, ver),
			fmt.Sprintf("db: %s", ctx.Project.DBPath),
			"hygiene: devdb doctor hygiene",
		}})
	}
	return doc
}

func argvFlag(name string) string {
	for i := 1; i < len(os.Args)-1; i++ {
		if os.Args[i] == name {
			return os.Args[i+1]
		}
	}
	return ""
}

func cmdFeedback(open opener) *cobra.Command {
	feedbackCmd := &cobra.Command{Use: "feedback", Short: "Feedback and observations"}
	feedbackCmd.AddCommand(cmdFeedbackAdd(open), cmdFeedbackList(open), cmdFeedbackShow(open),
		cmdFeedbackClose(open), cmdFeedbackAnnotate(open), cmdFeedbackImport(open))
	return feedbackCmd
}

func cmdFeedbackAdd(open opener) *cobra.Command {
	var role, category, severity, context string
	c := &cobra.Command{
		Use:   "add NOTE",
		Short: "Log feedback",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			if role == "" {
				return usageError("--role is required (user|model|codebase)")
			}
			id, err := feedback.Add(ctx.DB, feedback.AddInput{
				Role: role, Category: category, Severity: severity,
				Note: strings.Join(args, " "), Context: context, ModelID: ctx.ModelID,
			})
			if err != nil {
				return err
			}
			return ctx.Out.WriteResult(id, map[string]any{"kind": "feedback"})
		},
	}
	c.Flags().StringVar(&role, "role", "", "user|model|codebase")
	c.Flags().StringVar(&category, "category", "", "feedback category")
	c.Flags().StringVar(&severity, "severity", "", "info|low|med|high|critical")
	c.Flags().StringVar(&context, "context", "", "optional context")
	return c
}

func cmdFeedbackList(open opener) *cobra.Command {
	var st string
	c := &cobra.Command{
		Use:   "list",
		Short: "List feedback",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			if err := validateEnum("status", st, []string{"open", "closed", "all"}); err != nil {
				return err
			}
			rows, err := feedback.List(ctx.DB, st, effectiveListLimit(20))
			if err != nil {
				return err
			}
			return ctx.Out.PrintData(rows)
		},
	}
	c.Flags().StringVar(&st, "status", "open", "filter by status (open|closed|all)")
	return c
}

func cmdFeedbackShow(open opener) *cobra.Command {
	return &cobra.Command{
		Use:   "show ID",
		Short: "Show one feedback row",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			row, err := feedback.Show(ctx.DB, args[0])
			if err != nil {
				return notFoundError(err.Error())
			}
			return ctx.Out.PrintData(row)
		},
	}
}

func cmdFeedbackClose(open opener) *cobra.Command {
	var fix string
	c := &cobra.Command{
		Use:   "close ID",
		Short: "Close feedback",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			id, err := feedback.Close(ctx.DB, args[0], fix, ctx.ModelID)
			if err != nil {
				return err
			}
			return ctx.Out.WriteResult(id, map[string]any{"kind": "feedback", "action": "close"})
		},
	}
	c.Flags().StringVar(&fix, "proposed-fix", "", "how it was addressed")
	return c
}

func cmdPlan(open opener) *cobra.Command {
	plan := &cobra.Command{Use: "plan", Short: "Structured planning"}
	plan.AddCommand(
		cmdPlanCreate(open), cmdPlanList(open), cmdPlanShow(open), cmdPlanTree(open), cmdPlanStatus(open),
		cmdPlanScaffold(open), cmdPlanPromote(open), cmdPlanReconcile(open),
	)
	ms := &cobra.Command{Use: "milestone", Short: "Plan milestones"}
	ms.AddCommand(cmdPlanMilestoneAdd(open), cmdPlanMilestoneList(open), cmdPlanMilestoneStatus(open))
	plan.AddCommand(ms)
	item := &cobra.Command{Use: "item", Short: "Plan items"}
	item.AddCommand(cmdPlanItemAdd(open), cmdPlanItemList(open), cmdPlanItemShow(open),
		cmdPlanItemStart(open), cmdPlanItemPause(open), cmdPlanItemClose(open), cmdPlanItemStatus(open))
	plan.AddCommand(item)
	acc := &cobra.Command{Use: "acceptance", Short: "Acceptance criteria"}
	acc.AddCommand(cmdPlanAcceptanceAdd(open), cmdPlanAcceptanceMeet(open), cmdPlanAcceptanceBackfill(open))
	plan.AddCommand(acc)
	file := &cobra.Command{Use: "file", Short: "Scoped plan files"}
	file.AddCommand(cmdPlanFileAdd(open))
	plan.AddCommand(file)
	return plan
}

func cmdPlanCreate(open opener) *cobra.Command {
	var slug, body string
	c := &cobra.Command{
		Use:   "create TITLE",
		Short: "Create a structured plan",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			id, err := planning.CreatePlan(ctx.DB, planning.CreatePlanInput{
				Slug: slug, Title: strings.Join(args, " "), Body: body, ModelID: ctx.ModelID,
			})
			if err != nil {
				return err
			}
			return ctx.Out.WriteResult(id, map[string]any{"kind": "plan"})
		},
	}
	c.Flags().StringVar(&slug, "slug", "", "plan slug")
	c.Flags().StringVar(&body, "body", "", "plan body")
	return c
}

func cmdPlanList(open opener) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List plans",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			plans, err := planning.ListPlans(ctx.DB)
			if err != nil {
				return err
			}
			return ctx.Out.PrintData(plans)
		},
	}
}

func cmdPlanItemAdd(open opener) *cobra.Command {
	var planID, milestoneID, body, phase, step, memoryRef string
	var legacy bool
	c := &cobra.Command{
		Use:   "add TITLE",
		Short: "Add a plan item",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			title := strings.Join(args, " ")
			if planID != "" {
				resolved, err := planning.ResolvePlanID(ctx.DB, planID)
				if err != nil {
					return err
				}
				planID = resolved
			}
			var id string
			if legacy {
				id, err = planning.AddLegacyItem(ctx.DB, phase, step, title, body, memoryRef, ctx.ModelID)
			} else {
				id, err = planning.AddItem(ctx.DB, planning.AddItemInput{
					PlanID: planID, MilestoneID: milestoneID,
					Title: title, Body: body, MemoryRef: memoryRef, ModelID: ctx.ModelID,
				})
			}
			if err != nil {
				return err
			}
			return ctx.Out.WriteResult(id, map[string]any{"kind": "plan_item"})
		},
	}
	c.Flags().StringVar(&planID, "plan", "", "plan id")
	c.Flags().StringVar(&milestoneID, "milestone", "", "milestone id")
	c.Flags().StringVar(&body, "body", "", "item body")
	c.Flags().StringVar(&memoryRef, "memory-ref", "", "optional memory anchor such as MEMORY.md#section or memory/YYYY-MM-DD.md")
	c.Flags().BoolVar(&legacy, "legacy", false, "create legacy flat item (phase/step)")
	c.Flags().StringVar(&phase, "phase", "", "legacy phase label")
	c.Flags().StringVar(&step, "step", "", "legacy step label")
	return c
}

func cmdPlanItemShow(open opener) *cobra.Command {
	return &cobra.Command{
		Use:   "show ID",
		Short: "Show plan item detail",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			item, acc, err := planning.ShowItem(ctx.DB, args[0])
			if err != nil {
				return notFoundError(err.Error())
			}
			if ctx.Out.JSON {
				return ctx.Out.PrintData(map[string]any{"item": item, "acceptance": acc})
			}
			lines := []string{
				fmt.Sprintf("# %s", item.Title),
				fmt.Sprintf("status: %s · id: %s", item.Status, item.ID),
			}
			if item.Phase != "" {
				lines = append(lines, fmt.Sprintf("legacy: %s.%s", item.Phase, item.Step))
			}
			if item.MemoryRef != "" {
				lines = append(lines, fmt.Sprintf("memory_ref: %s", item.MemoryRef))
			}
			files, _ := planning.ListPlanFiles(ctx.DB, item.ID)
			if len(files) > 0 {
				lines = append(lines, "", "files:")
				for _, f := range files {
					lines = append(lines, fmt.Sprintf("- %s (%s)", f.Path, f.Role))
				}
			}
			if len(acc) > 0 {
				lines = append(lines, "", "acceptance:")
				for _, a := range acc {
					mark := "open"
					if a.Status == "met" {
						mark = "met"
					}
					lines = append(lines, fmt.Sprintf("- [%s] %d. %s [%s]", a.ID[:8], a.Ordinal, a.Criterion, mark))
				}
			}
			return ctx.Out.PrintData(output.HumanLines{Lines: lines})
		},
	}
}

func cmdPlanItemStart(open opener) *cobra.Command {
	return &cobra.Command{
		Use:   "start ID",
		Short: "Start work on a plan item",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			id, err := planning.StartItem(ctx.DB, args[0], ctx.ModelID)
			if err != nil {
				return err
			}
			item, acc, err := planning.ShowItem(ctx.DB, id)
			if err != nil {
				return err
			}
			for _, a := range acc {
				if a.Status == "open" {
					ctx.Out.Hint("unmet: [%s] %s", a.ID[:8], a.Criterion)
				}
			}
			ctx.Out.Hint("plan item %s · %s", id[:8], item.Title)
			return ctx.Out.WriteResult(id, map[string]any{"kind": "plan_item", "action": "start"})
		},
	}
}

func cmdPlanItemPause(open opener) *cobra.Command {
	var note string
	c := &cobra.Command{
		Use:   "pause ID",
		Short: "Pause in-flight work",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			id, err := planning.PauseItem(ctx.DB, args[0], note, ctx.ModelID)
			if err != nil {
				return err
			}
			return ctx.Out.WriteResult(id, map[string]any{"kind": "plan_item", "action": "pause"})
		},
	}
	c.Flags().StringVar(&note, "note", "", "required pause note")
	return c
}

func cmdPlanItemClose(open opener) *cobra.Command {
	var evidence, note string
	c := &cobra.Command{
		Use:   "close ID",
		Short: "Close a plan item when acceptance is met",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			id, err := planning.CloseItem(ctx.DB, args[0], combineEvidenceAndNote(evidence, note), ctx.ModelID)
			if err != nil {
				return &CLIError{Code: ExitInvalidValue, Message: err.Error(), Kind: "invalid_argument"}
			}
			return ctx.Out.WriteResult(id, map[string]any{"kind": "plan_item", "action": "close"})
		},
	}
	c.Flags().StringVar(&evidence, "evidence", "", "closure evidence (e.g. commit sha)")
	c.Flags().StringVar(&note, "note", "", "free-form annotation prepended to the status_log entry")
	return c
}

// combineEvidenceAndNote produces the status_log note for plan item close,
// matching the convention used by plan item status --note.
func combineEvidenceAndNote(evidence, note string) string {
	switch {
	case note != "" && evidence != "":
		return note + " — " + evidence
	case note != "":
		return note
	default:
		return evidence
	}
}

func cmdPlanAcceptanceAdd(open opener) *cobra.Command {
	var ordinal int
	var planItemID string
	c := &cobra.Command{
		Use:   "add CRITERION",
		Short: "Add acceptance criterion",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			if planItemID == "" {
				return usageError("--plan-item is required")
			}
			id, err := planning.AddAcceptance(ctx.DB, planItemID, strings.Join(args, " "), ctx.ModelID, ordinal)
			if err != nil {
				return err
			}
			return ctx.Out.WriteResult(id, map[string]any{"kind": "acceptance"})
		},
	}
	c.Flags().StringVar(&planItemID, "plan-item", "", "plan item id")
	c.Flags().IntVar(&ordinal, "ordinal", 0, "criterion order (auto when 0)")
	return c
}

func cmdPlanAcceptanceMeet(open opener) *cobra.Command {
	var evidence string
	c := &cobra.Command{
		Use:   "meet ID",
		Short: "Mark acceptance criterion met",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			if evidence == "" {
				return usageError("--evidence is required")
			}
			id, err := planning.MeetAcceptance(ctx.DB, args[0], evidence, ctx.ModelID)
			if err != nil {
				return err
			}
			return ctx.Out.WriteResult(id, map[string]any{"kind": "acceptance", "action": "meet"})
		},
	}
	c.Flags().StringVar(&evidence, "evidence", "", "evidence note or commit")
	return c
}

func cmdImport(open opener) *cobra.Command {
	var outputPath string
	var apply, replace, noArchive, force bool
	c := &cobra.Command{
		Use:   "python-db [PATH]",
		Short: "Inspect or import a legacy Python database",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			path := ctx.Project.DBPath
			if len(args) > 0 {
				path = args[0]
			}
			if apply {
				result, err := importer.ApplyInPlace(path, noArchive, force)
				if err != nil {
					if errors.Is(err, importer.ErrPythonBakAlreadyMigrated) {
						return fmt.Errorf("%w — sibling %s.python-bak has Go schema", err, path)
					}
					return err
				}
				if noArchive {
					ctx.Out.Hint("imported legacy database in place · backup at .devdb/development.db.python-bak (auto-archive skipped)")
				} else {
					ctx.Out.Hint("imported legacy database in place · backup at .devdb/development.db.python-bak · python-only rows archived to .devdb/archive-python-only/")
				}
				return ctx.Out.PrintData(result)
			}
			if outputPath != "" {
				result, err := importer.ImportPythonDB(path, outputPath, replace)
				if err != nil {
					return err
				}
				ctx.Out.Hint("imported %d tables into Go-native schema", len(result.Tables))
				return ctx.Out.PrintData(result)
			}
			info, err := importer.InspectPythonDB(path)
			if err != nil {
				return err
			}
			ctx.Out.Hint("legacy python db v%d · %d tables — use --output PATH or --apply to migrate", info.Version, info.Tables)
			return ctx.Out.PrintData(info)
		},
	}
	c.Flags().StringVarP(&outputPath, "output", "o", "", "write imported Go-native database to PATH")
	c.Flags().BoolVar(&apply, "apply", false, "migrate PATH in place (backs up to development.db.python-bak)")
	c.Flags().BoolVar(&replace, "replace", false, "overwrite non-empty destination when using --output")
	c.Flags().BoolVar(&noArchive, "no-archive-python-only", false, "with --apply, skip auto-archiving python-only tables to .devdb/archive-python-only/")
	c.Flags().BoolVar(&force, "force", false, "with --apply, ignore the .python-bak-already-migrated guard")
	importCmd := &cobra.Command{Use: "import", Short: "One-time data import"}
	importCmd.AddCommand(c)
	return importCmd
}

// examplesByCommand maps "noun verb" (or just "noun" / "help") to a
// multi-line example block. Surfaced via `devdb help <noun> <verb>
// --examples` so agents get real command shapes plus known gotchas instead
// of generic flag listings. Add an entry when a verb has a non-obvious
// flag, a common mistake, or a recently-fixed defect worth remembering.
var examplesByCommand = map[string]string{
	"help": `devdb help plan reconcile                # standard help for one verb
devdb help plan reconcile --examples     # real command shapes + gotchas
devdb help feedback close --examples     # artifact-naming template for --proposed-fix
devdb help archive run --examples        # cron-safe hygiene command
devdb help ledger housekeeping --examples`,
	"plan reconcile": `devdb plan reconcile                          # show drift in every plan
devdb plan reconcile --plan my-plan             # drift in one plan
devdb plan reconcile --plan my-plan --apply     # repair the drift

Note: there is no --dry-run flag. The verb's natural mode is dry-run; pass
--apply to repair. (Gotcha that bit triage on 2026-06-24.)`,
	"plan item close": `devdb plan item close <id> --evidence "<commit-or-note>"
devdb plan item close <id-prefix> --note "free-form annotation" --evidence "<commit>"

Closes the item and, when it is the last open item in its milestone or
plan, auto-cascades the parent statuses to done in a single transaction.
Run 'devdb plan acceptance meet' calls sequentially from one shell, not
in parallel — see skills/devdb/SKILL.md "Closure ritual".`,
	"plan acceptance meet": `devdb plan acceptance meet <criterion-id> --evidence "<commit-or-note>"

Run these sequentially from one shell, not in parallel from multiple
background processes. Parallel invocations from separate processes have
been observed to surface SQLITE_BUSY even with the in-process retry
budget; sequential keeps the commit window contention-free.`,
	"feedback close": `devdb feedback close <id> --proposed-fix "Resolved by <commit> which adds <artifact>: <one-line description>"

Name the commit + the artifact, not the closing agent's behavior. See
skills/devdb/SKILL.md "Closing feedback — name the artifact, not the behavior".`,
	"feedback list": `devdb feedback list --json                       # stable: returns [] on empty, never null
devdb feedback list --status open --json            # default status filter
devdb feedback list --status bogus                   # ERROR: --status: invalid value "bogus" (allowed: open, closed, all)

Empty --json output is always an array, so agents can iterate without a
nil-guard. Bogus filter values surface as explicit errors (exit code
ExitUsage) rather than silently producing an empty result. See feedback
[654b7b3e] for the original DX complaint.`,
	"archive run": `devdb archive run --dry-run            # preview what would move
devdb archive run --yes                # actually archive
devdb archive run --yes --vacuum       # archive + reclaim freed pages

Non-interactive by design: --yes suppresses the confirmation prompt,
--json is machine-readable. Cron entry:

  0 3 * * 0  cd /path/to/repo && devdb archive run --yes --json >> .devdb/archive.log 2>&1`,
	"ledger housekeeping": `devdb ledger housekeeping                 # run archive + reconcile + inventory scan + verify query
devdb ledger housekeeping --dry-run       # preview without writing
devdb ledger housekeeping --json          # machine-readable output

Bundles the recurring hygiene steps (archive run --yes, plan reconcile
--apply, inventory scan, verify query summary) into one command so the
weekly cron and end-of-session agents do not hand-craft the sequence.`,
}

func exampleForCommand(key string) string {
	if ex, ok := examplesByCommand[key]; ok {
		return ex
	}
	return fmt.Sprintf("(no examples registered for %q; add one to examplesByCommand in internal/cli/commands.go)", key)
}

func cmdHelp() *cobra.Command {
	var examples bool
	cmd := &cobra.Command{
		Use:   "help [command]",
		Short: "Show help for a command (no arg = global help)",
		Long: `devdb help <noun> <verb>             # standard help for one verb
devdb help <noun> <verb> --examples  # real command shapes + gotchas

Pass --examples after the verb name to print a curated example block
instead of the standard flag listing. Examples live in
internal/cli/commands.go (examplesByCommand) and are added when a verb
has a non-obvious gotcha worth remembering.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			root := cmd.Root()
			if examples {
				key := strings.Join(args, " ")
				fmt.Fprintln(cmd.OutOrStdout(), exampleForCommand(key))
				return nil
			}
			if len(args) == 0 {
				return root.Help()
			}
			target, _, err := root.Find(args)
			if err != nil || target == root {
				fmt.Fprintf(cmd.ErrOrStderr(), "unknown command %q\n", strings.Join(args, " "))
				return root.Help()
			}
			if target.Hidden || target.Name() == "help" {
				fmt.Fprintf(cmd.ErrOrStderr(), "unknown command %q\n", strings.Join(args, " "))
				return root.Help()
			}
			return target.Help()
		},
	}
	cmd.Flags().BoolVar(&examples, "examples", false, "print curated example block instead of standard help")
	return cmd
}

var flagRepo, flagDB string
var flagAll, flagVerbose bool
