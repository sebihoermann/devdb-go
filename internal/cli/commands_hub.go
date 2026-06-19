package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/sebihoermann/devdb-go/internal/domain/hub"
	"github.com/sebihoermann/devdb-go/internal/output"
	"github.com/spf13/cobra"
)

var (
	flagMetadataDB string
	flagRegistry   string
)

func cmdHub(openCtx opener) *cobra.Command {
	h := &cobra.Command{Use: "hub", Short: "Cross-project metadata hub"}
	h.PersistentFlags().StringVar(&flagMetadataDB, "metadata-db", "", "hub database path (default: ~/.devdb/metadata.db)")
	h.PersistentFlags().StringVar(&flagRegistry, "registry", "", "project registry path (default: ~/.devdb-projects)")
	h.AddCommand(
		cmdHubRegister(openCtx),
		cmdHubUnregister(openCtx),
		cmdHubList(openCtx),
		cmdHubSync(openCtx),
		cmdHubDashboard(openCtx),
		cmdHubProject(openCtx),
		cmdHubDoctor(openCtx),
		cmdHubAcross(openCtx),
	)
	return h
}

func cmdHubRegister(openCtx opener) *cobra.Command {
	var alias string
	var auto bool
	var scope string
	c := &cobra.Command{
		Use:   "register [PATH]",
		Short: "Register a project in the hub (or use --auto to walk a scope)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := openCtx(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if auto {
				registered, err := hub.AutoRegister(scope, flagRegistry, flagMetadataDB)
				if err != nil {
					return err
				}
				ctx.Out.Hint("auto-registered %d projects", len(registered))
				return ctx.Out.WriteResult(strings.Join(registered, ","), map[string]any{"registered": registered})
			}
			if len(args) != 1 {
				return fmt.Errorf("register requires PATH (or use --auto)")
			}
			p, err := hub.Register(args[0], alias, flagRegistry, flagMetadataDB)
			if err != nil {
				return err
			}
			ctx.Out.Hint("registered %s → %s", p.Alias, p.Root)
			return ctx.Out.WriteResult(p.Alias, map[string]any{"root": p.Root})
		},
	}
	c.Flags().StringVar(&alias, "alias", "", "project alias (default: directory name)")
	c.Flags().BoolVar(&auto, "auto", false, "walk SCOPE and register every .devdb/development.db found")
	c.Flags().StringVar(&scope, "scope", ".", "scope directory for --auto (default: cwd)")
	return c
}

func cmdHubUnregister(openCtx opener) *cobra.Command {
	c := &cobra.Command{
		Use:   "unregister ALIAS_OR_PATH",
		Short: "Remove a project from the hub by alias or root path",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := openCtx(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := hub.Unregister(args[0], flagRegistry, flagMetadataDB); err != nil {
				return err
			}
			ctx.Out.Hint("unregistered %s", args[0])
			return ctx.Out.WriteResult(args[0], map[string]any{"removed": true})
		},
	}
	return c
}

func cmdHubList(openCtx opener) *cobra.Command {
	var refresh bool
	c := &cobra.Command{
		Use:   "list",
		Short: "List registered projects",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := openCtx(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			entries, err := hub.List(flagRegistry, flagMetadataDB, refresh)
			if err != nil {
				return err
			}
			if ctx.Out.JSON {
				return ctx.Out.PrintData(entries)
			}
			var lines []string
			for _, e := range entries {
				status := e.Status
				if status == "" {
					status = "unsynced"
				}
				line := fmt.Sprintf("%s  %s  [%s]", e.Alias, e.Root, status)
				if e.SyncedAt != "" {
					line += "  synced " + e.SyncedAt
				}
				lines = append(lines, line)
			}
			if len(lines) == 0 {
				lines = []string{"no registered projects"}
			}
			return ctx.Out.PrintData(output.HumanLines{Lines: lines})
		},
	}
	c.Flags().BoolVar(&refresh, "refresh", false, "re-sync projects with newer source databases")
	return c
}

func cmdHubSync(openCtx opener) *cobra.Command {
	var strict, watch bool
	var interval float64
	var iterations int
	c := &cobra.Command{
		Use:   "sync",
		Short: "Refresh hub snapshots from all registered projects",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := openCtx(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()

			runSync := func() (hub.SyncResult, error) {
				return hub.SyncAll(flagRegistry, flagMetadataDB, strict)
			}

			if !watch {
				res, err := runSync()
				if err != nil {
					return err
				}
				if ctx.Out.JSON {
					return ctx.Out.PrintData(res)
				}
				ctx.Out.Hint("sync %s: %s seen=%d updated=%d failed=%d",
					res.RunID, res.Status, res.ProjectsSeen, res.ProjectsUpdated, res.ProjectsFailed)
				for _, e := range res.Errors {
					ctx.Out.Hint("  %s", e)
				}
				if strict && res.ProjectsFailed > 0 {
					return &CLIError{Code: ExitGeneral, Message: "sync failed in strict mode", Kind: "sync_error"}
				}
				return nil
			}

			tick := 0
			for {
				tick++
				res, err := runSync()
				if err != nil {
					return err
				}
				if ctx.Out.JSON {
					if err := ctx.Out.PrintData(res); err != nil {
						return err
					}
				} else {
					ctx.Out.Hint("watch %d: sync %s: %s seen=%d updated=%d failed=%d",
						tick, res.RunID, res.Status, res.ProjectsSeen, res.ProjectsUpdated, res.ProjectsFailed)
					for _, e := range res.Errors {
						ctx.Out.Hint("  %s", e)
					}
				}
				if iterations > 0 && tick >= iterations {
					break
				}
				if interval > 0 {
					time.Sleep(time.Duration(interval * float64(time.Second)))
				} else {
					break
				}
			}
			return nil
		},
	}
	c.Flags().BoolVar(&strict, "strict", false, "exit non-zero when any project fails")
	c.Flags().BoolVar(&watch, "watch", false, "repeat sync on an interval")
	c.Flags().Float64Var(&interval, "interval", 60, "seconds between watch iterations")
	c.Flags().IntVar(&iterations, "iterations", 0, "watch iteration limit (0 = infinite until interrupted)")
	return c
}

func cmdHubDashboard(openCtx opener) *cobra.Command {
	var view string
	var attentionOnly bool
	c := &cobra.Command{
		Use:   "dashboard",
		Short: "Compact cross-project work/quality/delivery view",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := openCtx(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			rows, err := hub.Dashboard(flagRegistry, flagMetadataDB, view, attentionOnly)
			if err != nil {
				return err
			}
			if ctx.Out.JSON {
				return ctx.Out.PrintData(rows)
			}
			lines := formatDashboard(rows, view)
			if len(lines) == 0 {
				lines = []string{"no projects match"}
			}
			return ctx.Out.PrintData(output.HumanLines{Lines: lines})
		},
	}
	c.Flags().StringVar(&view, "view", "summary", "summary|work|delivery|quality")
	c.Flags().BoolVar(&attentionOnly, "attention-only", false, "only projects needing attention")
	return c
}

func formatDashboard(rows []hub.DashboardRow, view string) []string {
	var lines []string
	for _, r := range rows {
		switch view {
		case "work":
			lines = append(lines, fmt.Sprintf("%s  %s  in_progress=%d open=%d feedback=%d score=%d",
				r.Alias, r.WorkStatus, r.InProgress, r.OpenItems, r.OpenFeedback, r.AttentionScore))
		case "delivery":
			dirty := ""
			if r.GitDirty {
				dirty = " dirty"
			}
			lines = append(lines, fmt.Sprintf("%s  %s%s  branch=%s  %s",
				r.Alias, r.WorkStatus, dirty, r.GitBranch, r.StatusReason))
		case "quality":
			lines = append(lines, fmt.Sprintf("%s  findings=%d stale_arch=%d verify=%s score=%d",
				r.Alias, r.OpenHighFinding, r.StaleArch, r.Verification, r.AttentionScore))
		default:
			reason := r.StatusReason
			if reason == "" {
				reason = r.WorkStatus
			}
			lines = append(lines, fmt.Sprintf("%s  [%s] score=%d  %s",
				r.Alias, r.Status, r.AttentionScore, reason))
		}
	}
	return lines
}

func cmdHubProject(openCtx opener) *cobra.Command {
	c := &cobra.Command{
		Use:   "project ALIAS_OR_PATH",
		Short: "Show one registered project's hub detail",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := openCtx(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			detail, err := hub.Project(flagRegistry, flagMetadataDB, args[0])
			if err != nil {
				return &CLIError{Code: ExitNotFound, Message: err.Error(), Kind: "not_found"}
			}
			if ctx.Out.JSON {
				return ctx.Out.PrintData(detail)
			}
			s := detail.Snapshot
			lines := []string{
				fmt.Sprintf("%s  %s", detail.Alias, detail.Root),
				fmt.Sprintf("status=%s synced=%s work=%s score=%d",
					detail.Status, detail.SyncedAt, s.WorkStatus, s.AttentionScore),
			}
			if s.InFlightTitle != "" {
				lines = append(lines, "in flight: "+s.InFlightTitle)
			}
			if len(detail.Attention) > 0 {
				lines = append(lines, "attention:")
				for _, a := range detail.Attention {
					lines = append(lines, fmt.Sprintf("  [%s] %s: %s", a.Severity, a.Kind, a.Title))
				}
			}
			return ctx.Out.PrintData(output.HumanLines{Lines: lines})
		},
	}
	return c
}

func cmdHubDoctor(openCtx opener) *cobra.Command {
	var project string
	c := &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose hub sync freshness and dirty source markers",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := openCtx(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			rows, err := hub.Doctor(flagRegistry, flagMetadataDB, project)
			if err != nil {
				return err
			}
			if ctx.Out.JSON {
				return ctx.Out.PrintData(rows)
			}
			var lines []string
			for _, r := range rows {
				line := fmt.Sprintf("%s  freshness=%s hub=%s", r.Alias, r.FreshnessStatus, r.HubStatus)
				if r.RecommendedCmd != "" {
					line += " → " + r.RecommendedCmd
				}
				if r.LastSyncError != "" {
					line += " (" + r.LastSyncError + ")"
				}
				lines = append(lines, line)
			}
			if len(lines) == 0 {
				lines = []string{"no registered projects"}
			}
			return ctx.Out.PrintData(output.HumanLines{Lines: lines})
		},
	}
	c.Flags().StringVar(&project, "project", "", "filter to one alias or path")
	return c
}

func cmdHubAcross(openCtx opener) *cobra.Command {
	var keyword, category string
	c := &cobra.Command{
		Use:   "across QUERY",
		Short: "Run a built-in cross-project query",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := openCtx(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			rows, err := hub.Across(hub.AcrossOptions{
				Query: args[0], Keyword: keyword, Category: category, Registry: flagRegistry,
			})
			if err != nil {
				return &CLIError{Code: ExitInvalidValue, Message: err.Error(), Kind: "invalid_argument"}
			}
			if ctx.Out.JSON {
				return ctx.Out.PrintData(rows)
			}
			if len(rows) == 0 {
				return ctx.Out.PrintData("no rows")
			}
			var lines []string
			for _, row := range rows {
				lines = append(lines, fmt.Sprintf("%s  %v", row["project"], row))
			}
			return ctx.Out.PrintData(output.HumanLines{Lines: lines})
		},
	}
	c.Flags().StringVar(&keyword, "keyword", "", "filter for similar-feedback")
	c.Flags().StringVar(&category, "category", "", "filter for similar-feedback")
	return c
}
