package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/sebihoermann/devdb-go/internal/domain/planning"
	"github.com/sebihoermann/devdb-go/internal/output"
	"github.com/spf13/cobra"
)

func cmdPlanScaffold(open opener) *cobra.Command {
	var slug, body, mode, outputPath string
	var milestoneCount int
	var skipAcceptance bool
	var withImplementItems bool
	c := &cobra.Command{
		Use:   "scaffold TITLE",
		Short: "Create plan skeleton and HTML artifact",
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
			result, err := planning.ScaffoldPlan(ctx.DB, planning.ScaffoldPlanInput{
				Title: strings.Join(args, " "), Slug: slug, Body: body, Mode: mode,
				MilestoneCount: milestoneCount, OutputPath: outputPath,
				RepoRoot: ctx.Project.RepoRoot, SkipAcceptance: skipAcceptance,
				ModelID: ctx.ModelID, WithImplementItems: withImplementItems,
			})
			if err != nil {
				if errors.Is(err, planning.ErrSlugExists) {
					return &CLIError{Code: ExitGeneral, Message: err.Error(), Kind: "invalid_argument"}
				}
				if errors.Is(err, planning.ErrInvalidMode) {
					return &CLIError{Code: ExitUsage, Message: err.Error(), Kind: "invalid_argument"}
				}
				return err
			}
			ctx.Out.Hint("artifact: %s", result.Artifact)
			ctx.Out.Hint("next: devdb plan item start <id> · artifact at %s", result.Artifact)
			meta := map[string]any{
				"kind": "plan", "action": "scaffold",
				"slug": result.Slug, "artifact": result.Artifact, "milestones": result.Milestones,
			}
			return ctx.Out.WriteResult(result.PlanID, meta)
		},
	}
	c.Flags().StringVar(&slug, "slug", "", "plan slug (auto from title)")
	c.Flags().StringVar(&body, "body", "", "plan description")
	c.Flags().StringVar(&mode, "mode", "implement", "design|implement")
	c.Flags().IntVar(&milestoneCount, "milestones", 4, "number of milestones")
	c.Flags().StringVar(&outputPath, "output", "", "HTML artifact path (default: docs/<slug>-implementation-plan.html)")
	c.Flags().BoolVar(&skipAcceptance, "skip-acceptance", false, "omit default acceptance criteria")
	c.Flags().BoolVar(&withImplementItems, "with-implement-items", false, "also create one auto 'Implement/Design Milestone N' item per milestone (legacy shape; default off)")
	return c
}

func cmdPlanPromote(open opener) *cobra.Command {
	var planRef string
	c := &cobra.Command{
		Use:   "promote",
		Short: "Promote design-scaffolded items to implement mode",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			if planRef == "" {
				return usageError("--plan is required")
			}
			result, err := planning.PromotePlan(ctx.DB, planRef, ctx.ModelID)
			if err != nil {
				if errors.Is(err, planning.ErrPlanNotFound) {
					return notFoundError("plan not found")
				}
				return err
			}
			ctx.Out.Hint("promoted %d title(s) and %d acceptance row(s) to implement mode",
				result.TitlesUpdated, result.AcceptanceUpdated)
			return ctx.Out.WriteResult(result.PlanID, map[string]any{
				"kind": "plan", "action": "promote",
				"plan": planRef, "titles_updated": result.TitlesUpdated,
				"acceptance_updated": result.AcceptanceUpdated,
			})
		},
	}
	c.Flags().StringVar(&planRef, "plan", "", "plan slug or id")
	return c
}

func cmdPlanReconcile(open opener) *cobra.Command {
	var planRef string
	var apply bool
	c := &cobra.Command{
		Use:   "reconcile",
		Short: "Detect or repair plan-tree status drift",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			result, err := planning.ReconcilePlans(ctx.DB, planRef, apply)
			if err != nil {
				if errors.Is(err, planning.ErrPlanNotFound) {
					return notFoundError("plan not found")
				}
				return err
			}
			if ctx.Out.JSON {
				return ctx.Out.PrintData(result)
			}
			if result.Drift.IsEmpty() {
				return ctx.Out.PrintData(output.HumanLines{Lines: []string{"no plan-tree drift detected"}})
			}
			if !apply {
				lines := []string{
					fmt.Sprintf("drift detected: %d milestone(s), %d plan(s)",
						len(result.Drift.Milestones), len(result.Drift.Plans)),
				}
				for _, item := range result.Drift.Milestones {
					lines = append(lines, fmt.Sprintf("- milestone M%d [%s] %s -> %s",
						item.Number, item.PlanSlug, item.CurrentStatus, item.ExpectedStatus))
				}
				for _, item := range result.Drift.Plans {
					lines = append(lines, fmt.Sprintf("- plan %s %s -> %s",
						item.Slug, item.CurrentStatus, item.ExpectedStatus))
				}
				lines = append(lines, "run with --apply to repair")
				return ctx.Out.PrintData(output.HumanLines{Lines: lines})
			}
			applied := result.Applied
			if applied == nil {
				applied = &planning.AppliedReconcile{}
			}
			lines := []string{
				fmt.Sprintf("reconciled %d milestone(s), %d plan(s)",
					len(applied.Milestones), len(applied.Plans)),
			}
			return ctx.Out.PrintData(output.HumanLines{Lines: lines})
		},
	}
	c.Flags().StringVar(&planRef, "plan", "", "limit to one plan slug or id")
	c.Flags().BoolVar(&apply, "apply", false, "repair drifted statuses")
	return c
}

func cmdPlanDelete(open opener) *cobra.Command {
	var dryRun, force bool
	c := &cobra.Command{
		Use:   "delete SLUG_OR_ID",
		Short: "Delete a plan and its whole subtree",
		Long: "Removes the plan cascade in one transaction (acceptance criteria, item files, " +
			"status log, items, milestones, then the plan). Refuses when items are in_progress " +
			"unless --force. Preview with --dry-run: prints the cascade counts without writing. " +
			"Replaces the hand-ordered raw-SQL cascade recipe (skill pitfall 10).",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			res, err := planning.DeletePlan(ctx.DB, args[0], planning.DeletePlanInput{Force: force, DryRun: dryRun})
			if err != nil {
				if errors.Is(err, planning.ErrPlanNotFound) {
					return notFoundError("plan not found: " + args[0])
				}
				if errors.Is(err, planning.ErrPlanInProgress) {
					return &CLIError{
						Code:       ExitInvalidValue,
						Message:    fmt.Sprintf("plan %s has %d in_progress item(s) — an agent may be mid-task", res.Slug, res.InProgressItems),
						Suggestion: "pause or close the items first, or pass --force",
						Kind:       "invalid_argument",
					}
				}
				return err
			}
			if ctx.Out.JSON {
				return ctx.Out.PrintData(res)
			}
			verb := "deleted"
			if dryRun {
				verb = "would delete"
			}
			return ctx.Out.PrintData(output.HumanLines{Lines: []string{
				fmt.Sprintf("%s plan %s (%s): %d acceptance, %d files, %d status-log rows, %d items, %d milestones",
					verb, res.Title, res.Slug, res.Acceptance, res.Files, res.StatusLog, res.Items, res.Milestones),
			}})
		},
	}
	c.Flags().BoolVar(&dryRun, "dry-run", false, "preview cascade counts without writing")
	c.Flags().BoolVar(&force, "force", false, "delete even when items are in_progress")
	return c
}

func cmdPlanAcceptanceList(open opener) *cobra.Command {
	var planRef, itemRef string
	var openOnly bool
	c := &cobra.Command{
		Use:   "list",
		Short: "List acceptance criteria",
		Long: "List acceptance criteria, for one plan item (--plan-item) or for a whole\n" +
			"plan (--plan). This is the read verb the group never had: the only way\n" +
			"to see criteria used to be `plan item show`.",
		Example: `  devdb plan acceptance list --plan-item 2b718220
  devdb plan acceptance list --plan devdb-go-improvements-2026-11
  devdb plan acceptance list --plan devdb-go-improvements-2026-11 --open
  devdb plan acceptance list --plan-item 2b718220 --json`,
		Args: func(cmd *cobra.Command, args []string) error {
			// An unexpected positional argument is a typo, not a filter. Reject
			// it so `plan acceptance list <id>` cannot silently list everything.
			if len(args) > 0 {
				return usageError(fmt.Sprintf("unexpected argument %q (use --plan-item ID or --plan SLUG)", args[0]))
			}
			if planRef == "" && itemRef == "" {
				return usageError("one of --plan-item or --plan is required")
			}
			if planRef != "" && itemRef != "" {
				return usageError("--plan-item and --plan are mutually exclusive")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}

			var rows []planning.AcceptanceRow
			if itemRef != "" {
				rows, err = planning.ListAcceptanceByItem(ctx.DB, itemRef)
			} else if openOnly {
				rows, err = planning.ListAcceptanceOpen(ctx.DB, planRef)
			} else {
				rows, err = planning.ListAcceptanceByPlan(ctx.DB, planRef)
			}
			if err != nil {
				if errors.Is(err, planning.ErrPlanNotFound) || strings.Contains(err.Error(), "not found") {
					return notFoundError(err.Error())
				}
				return err
			}

			if ctx.Out.JSON {
				return ctx.Out.PrintData(rows)
			}
			if len(rows) == 0 {
				return ctx.Out.PrintData(output.EmptyText("no acceptance criteria"))
			}
			return ctx.Out.PrintData(output.HumanLines{Lines: acceptanceLines(rows)})
		},
	}
	c.Flags().StringVar(&planRef, "plan", "", "plan id or slug")
	c.Flags().StringVar(&itemRef, "plan-item", "", "plan item id (prefix ok)")
	c.Flags().BoolVar(&openOnly, "open", false, "only criteria that are not met yet (with --plan)")
	return c
}

// acceptanceLines renders criteria compactly: status marker, id prefix,
// ordinal, criterion, and the met evidence when present.
func acceptanceLines(rows []planning.AcceptanceRow) []string {
	lines := make([]string, 0, len(rows))
	for _, r := range rows {
		marker := "[ ]"
		if r.Status == "met" {
			marker = "[x]"
		}
		prefix := r.ID
		if len(prefix) > 8 {
			prefix = prefix[:8]
		}
		item := ""
		if r.PlanItemTitle != "" {
			item = fmt.Sprintf(" (%s)", truncateRunes(r.PlanItemTitle, 40))
		}
		line := fmt.Sprintf("%s %s %d. %s%s", marker, prefix, r.Ordinal, r.Criterion, item)
		if r.Evidence != "" {
			line += " — " + truncateRunes(r.Evidence, 60)
		}
		lines = append(lines, line)
	}
	return lines
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func cmdPlanAcceptanceBackfill(open opener) *cobra.Command {
	var milestone, specPath string
	c := &cobra.Command{
		Use:   "backfill",
		Short: "Backfill acceptance criteria from a Markdown spec",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			if milestone == "" {
				return usageError("--milestone is required (e.g. M1)")
			}
			if specPath == "" {
				return usageError("--spec is required")
			}
			count, err := planning.BackfillAcceptanceFromSpec(ctx.DB, milestone, specPath, ctx.ModelID)
			if err != nil {
				if errors.Is(err, planning.ErrSpecFileNotFound) {
					return notFoundError(err.Error())
				}
				return err
			}
			if ctx.Out.JSON {
				return ctx.Out.PrintData(map[string]int{"created": count})
			}
			return ctx.Out.PrintData(output.HumanLines{
				Lines: []string{fmt.Sprintf("created %d acceptance criteria", count)},
			})
		},
	}
	c.Flags().StringVar(&milestone, "milestone", "", "milestone heading (e.g. M1)")
	c.Flags().StringVar(&specPath, "spec", "", "path to Markdown spec file")
	return c
}
