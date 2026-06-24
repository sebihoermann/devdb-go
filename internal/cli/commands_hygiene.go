package cli

import (
	"fmt"

	"github.com/sebihoermann/devdb-go/internal/app"
	"github.com/sebihoermann/devdb-go/internal/domain/analytics"
	"github.com/sebihoermann/devdb-go/internal/domain/archive"
	"github.com/sebihoermann/devdb-go/internal/domain/inventory"
	"github.com/sebihoermann/devdb-go/internal/domain/planning"
	"github.com/sebihoermann/devdb-go/internal/domain/verification"
	"github.com/sebihoermann/devdb-go/internal/output"
	"github.com/spf13/cobra"
)

func cmdArchive(open opener) *cobra.Command {
	arch := &cobra.Command{Use: "archive", Short: "Archive historical ledger rows"}
	arch.AddCommand(cmdArchiveRun(open), cmdArchiveList(open), cmdArchiveRestore(open), cmdArchiveGC(open))
	return arch
}

func cmdArchiveRun(open opener) *cobra.Command {
	var table string
	var sessionHours, keepSnapshots int
	var dryRun, yes, vacuum bool
	c := &cobra.Command{
		Use:   "run",
		Short: "Move closed/resolved rows into archive_entries",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			res, err := archive.Run(ctx.DB, archive.RunOptions{
				SessionHours: sessionHours, KeepSnapshots: keepSnapshots,
				Table: table, DryRun: dryRun, Yes: yes, Vacuum: vacuum,
			})
			if err != nil {
				return &CLIError{Code: ExitInvalidValue, Message: err.Error(), Kind: "invalid_argument"}
			}
			if ctx.Out.JSON {
				return ctx.Out.PrintData(res)
			}
			if dryRun {
				lines := []string{fmt.Sprintf("would archive %d rows:", res.WouldArchiveTotal)}
				for t, n := range res.ByTable {
					if n > 0 {
						lines = append(lines, fmt.Sprintf("  %s %d", t, n))
					}
				}
				return ctx.Out.PrintData(output.HumanLines{Lines: lines})
			}
			lines := []string{fmt.Sprintf("archived %d rows:", res.ArchivedTotal)}
			for t, n := range res.ByTable {
				if n > 0 {
					lines = append(lines, fmt.Sprintf("  %s %d", t, n))
				}
			}
			if vacuum {
				ctx.Out.Hint("vacuumed")
			}
			return ctx.Out.PrintData(output.HumanLines{Lines: lines})
		},
	}
	c.Flags().StringVar(&table, "table", "", "scope to one source table")
	c.Flags().IntVar(&sessionHours, "session-hours", 24, "keep rows from the last N hours")
	c.Flags().IntVar(&keepSnapshots, "keep-snapshots", 3, "loc_snapshots retention count")
	c.Flags().BoolVar(&dryRun, "dry-run", false, "preview without writing")
	c.Flags().BoolVar(&yes, "yes", false, "skip confirmation")
	c.Flags().BoolVar(&vacuum, "vacuum", false, "VACUUM after archive")
	return c
}

func cmdArchiveList(open opener) *cobra.Command {
	var table, since, until string
	var limit int
	c := &cobra.Command{
		Use:   "list",
		Short: "List archive entries",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			rows, err := archive.List(ctx.DB, archive.ListFilter{
				Table: table, Since: since, Until: until, Limit: applyAllLimit(limit),
			})
			if err != nil {
				return err
			}
			if !ctx.Out.JSON && len(rows) == 0 {
				ctx.Out.Hint("no archive entries match")
			}
			return ctx.Out.PrintData(rows)
		},
	}
	c.Flags().StringVar(&table, "table", "", "filter by source table")
	c.Flags().StringVar(&since, "since", "", "archived_at >= ISO timestamp")
	c.Flags().StringVar(&until, "until", "", "archived_at <= ISO timestamp")
	c.Flags().IntVar(&limit, "limit", 50, "max rows")
	return c
}

func cmdArchiveRestore(open opener) *cobra.Command {
	var id, sourceTable, sourceID, table, since, until string
	var keepArchive bool
	c := &cobra.Command{
		Use:   "restore",
		Short: "Restore rows from archive_entries",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			res, err := archive.Restore(ctx.DB, archive.RestoreOptions{
				ID: id, SourceTable: sourceTable, SourceID: sourceID,
				Table: table, Since: since, Until: until, KeepArchive: keepArchive,
			})
			if err != nil {
				return &CLIError{Code: ExitUsage, Message: err.Error(), Kind: "missing_argument"}
			}
			if ctx.Out.JSON {
				return ctx.Out.PrintData(res)
			}
			return ctx.Out.PrintData(output.HumanLines{Lines: []string{
				fmt.Sprintf("restored %d · skipped %d · archive entries deleted %d",
					res.Restored, res.SkippedAlreadyPresent, res.ArchiveEntriesDeleted),
			}})
		},
	}
	c.Flags().StringVar(&id, "id", "", "archive entry id")
	c.Flags().StringVar(&sourceTable, "source-table", "", "filter source table")
	c.Flags().StringVar(&sourceID, "source-id", "", "filter source id")
	c.Flags().StringVar(&table, "table", "", "alias for --source-table")
	c.Flags().StringVar(&since, "since", "", "archived_at >= ISO timestamp")
	c.Flags().StringVar(&until, "until", "", "archived_at <= ISO timestamp")
	c.Flags().BoolVar(&keepArchive, "keep-archive", false, "leave archive row after restore")
	return c
}

func cmdArchiveGC(open opener) *cobra.Command {
	var olderThan int
	var dryRun bool
	c := &cobra.Command{
		Use:   "gc",
		Short: "Prune stale open feedback and old dismissed reminders/tasks",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			res, err := archive.GC(ctx.DB, archive.GCOptions{OlderThanDays: olderThan, DryRun: dryRun})
			if err != nil {
				return err
			}
			if ctx.Out.JSON {
				return ctx.Out.PrintData(res)
			}
			if dryRun {
				return ctx.Out.PrintData(output.HumanLines{Lines: []string{
					fmt.Sprintf("would close %d feedback (> %dd)", res.FeedbackToClose, res.OlderThanDays),
					fmt.Sprintf("would wontfix %d findings on missing files", res.FindingsToWontfix),
					fmt.Sprintf("would archive %d reminders · %d tasks", res.RemindersToArchive, res.TasksToArchive),
					fmt.Sprintf("stale architecture notes: %d", res.StaleArchNotes),
				}})
			}
			return ctx.Out.PrintData(output.HumanLines{Lines: []string{
				fmt.Sprintf("closed %d feedback · wontfix %d findings", res.FeedbackClosed, res.FindingsResolved),
				fmt.Sprintf("archived %d reminders · %d tasks", res.RemindersArchived, res.TasksArchived),
			}})
		},
	}
	c.Flags().IntVar(&olderThan, "older-than", 30, "age threshold in days")
	c.Flags().BoolVar(&dryRun, "dry-run", false, "preview without writing")
	return c
}

func cmdAnalytics(open opener) *cobra.Command {
	an := &cobra.Command{Use: "analytics", Short: "CLI telemetry"}
	an.AddCommand(cmdAnalyticsMissed(open), cmdAnalyticsSummary(open))
	return an
}

func cmdAnalyticsMissed(open opener) *cobra.Command {
	var since string
	var limit int
	c := &cobra.Command{
		Use:   "missed",
		Short: "List recent failed devdb invocations",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			rows, err := analytics.ListMissedCalls(ctx.DB, since, applyAllLimit(limit))
			if err != nil {
				return err
			}
			if ctx.Out.JSON {
				return ctx.Out.PrintData(rows)
			}
			lines := make([]string, 0, len(rows))
			for _, r := range rows {
				raw := r.RawArgv
				if len(raw) > 60 {
					raw = raw[:57] + "..."
				}
				ts := r.CreatedAt
				if len(ts) > 19 {
					ts = ts[:19]
				}
				lines = append(lines, fmt.Sprintf("%s · %s · %s", ts, r.FailureKind, raw))
			}
			return ctx.Out.PrintData(output.HumanLines{Lines: lines})
		},
	}
	c.Flags().StringVar(&since, "since", "", "ISO timestamp (default: 7 days ago)")
	c.Flags().IntVar(&limit, "limit", 50, "max rows")
	return c
}

func cmdAnalyticsSummary(open opener) *cobra.Command {
	var since string
	var windowDays int
	c := &cobra.Command{
		Use:   "summary",
		Short: "Grouped summary of failed invocation patterns",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			sum, err := analytics.MissedSummary(ctx.DB, since, windowDays)
			if err != nil {
				return err
			}
			if ctx.Out.JSON {
				return ctx.Out.PrintData(sum)
			}
			lines := []string{fmt.Sprintf("total misses (last %dd): %d", sum.WindowDays, sum.Total)}
			if len(sum.TopFailureKinds) > 0 {
				lines = append(lines, "", "top failure kinds:")
				for _, k := range sum.TopFailureKinds {
					lines = append(lines, fmt.Sprintf("  %s %dx", k.Kind, k.Count))
				}
			}
			if len(sum.TopCommands) > 0 {
				lines = append(lines, "", "top commands:")
				for _, c := range sum.TopCommands {
					lines = append(lines, fmt.Sprintf("  %s %dx", c.Command, c.Count))
				}
			}
			return ctx.Out.PrintData(output.HumanLines{Lines: lines})
		},
	}
	c.Flags().StringVar(&since, "since", "", "ISO timestamp (default: 7 days ago)")
	c.Flags().IntVar(&windowDays, "window-days", 7, "display window in days")
	return c
}

func cmdDoctorHygiene(open opener) *cobra.Command {
	return &cobra.Command{
		Use:   "hygiene",
		Short: "Per-repo CLI hygiene (missed calls, arch note pressure)",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			rep, err := analytics.Hygiene(ctx.DB)
			if err != nil {
				return err
			}
			if ctx.Out.JSON {
				return ctx.Out.PrintData(rep)
			}
			lines := []string{fmt.Sprintf("CLI hygiene (last 7d): %d missed call(s)", rep.MissedCalls7d)}
			for _, k := range rep.TopFailureKinds {
				lines = append(lines, fmt.Sprintf("  %s %dx", k.Kind, k.Count))
			}
			if len(rep.TopCommands) > 0 {
				lines = append(lines, "top commands:")
				for _, c := range rep.TopCommands {
					lines = append(lines, fmt.Sprintf("  %s %dx", c.Command, c.Count))
				}
			}
			lines = append(lines, fmt.Sprintf("active architecture notes: %d", rep.ActiveArchNotes))
			for _, tip := range rep.Recommendations {
				lines = append(lines, "tip: "+tip)
			}
			return ctx.Out.PrintData(output.HumanLines{Lines: lines})
		},
	}
}

// cmdLedger returns the "ledger" noun with bundled hygiene verbs. The
// "housekeeping" verb is the cron-safe, end-of-session batch that combines
// archive run --yes, plan reconcile --apply, inventory scan, and an
// optional verify query so weekly cron entries and end-of-session agents
// do not have to hand-craft the sequence (the friction called out in
// docs/workflow-review-2026-06-24.md).
func cmdLedger(open opener) *cobra.Command {
	ledger := &cobra.Command{Use: "ledger", Short: "Bundled hygiene operations on the devdb ledger"}
	ledger.AddCommand(cmdLedgerHousekeeping(open))
	return ledger
}

type housekeepingStep struct {
	Name   string `json:"name"`
	Status string `json:"status"` // "ran" | "would-run" | "failed" | "skipped"
	Detail string `json:"detail,omitempty"`
}

type housekeepingResult struct {
	DryRun bool               `json:"dry_run"`
	Steps  []housekeepingStep `json:"steps"`
}

func runHousekeepingSteps(ctx *app.Context, dryRun bool, verifyCommand, verifyScope string, scanPaths []string) housekeepingResult {
	res := housekeepingResult{DryRun: dryRun}

	// Step 1: archive run. Yes=true when actually writing so the user does
	// not get a confirmation prompt mid-batch; DryRun=true for preview.
	archRes, archErr := archive.Run(ctx.DB, archive.RunOptions{
		SessionHours:  24,
		KeepSnapshots: 3,
		DryRun:        dryRun,
		Yes:           !dryRun,
	})
	step := housekeepingStep{Name: "archive-run"}
	switch {
	case archErr != nil:
		step.Status = "failed"
		step.Detail = archErr.Error()
	case dryRun:
		step.Status = "would-run"
		step.Detail = fmt.Sprintf("would archive %d rows", archRes.WouldArchiveTotal)
	default:
		step.Status = "ran"
		step.Detail = fmt.Sprintf("archived %d rows", archRes.ArchivedTotal)
	}
	res.Steps = append(res.Steps, step)

	// Step 2: plan reconcile (apply when not dry-run). Drift counts only;
	// a non-zero count is informational, not an error.
	recRes, recErr := planning.ReconcilePlans(ctx.DB, "", !dryRun)
	step = housekeepingStep{Name: "plan-reconcile"}
	switch {
	case recErr != nil:
		step.Status = "failed"
		step.Detail = recErr.Error()
	default:
		step.Status = "ran"
		drift := recRes.Drift.DriftCount()
		if recRes.Applied != nil {
			step.Detail = fmt.Sprintf("%d drift rows repaired", drift)
		} else {
			step.Detail = fmt.Sprintf("%d drift rows detected (no --apply)", drift)
		}
	}
	res.Steps = append(res.Steps, step)

	// Step 3: inventory scan. Skip under --dry-run (the scan is read-only
	// and would always be reported as "would-run"; the archive + reconcile
	// preview is what the operator wants to see first).
	if dryRun {
		res.Steps = append(res.Steps, housekeepingStep{Name: "inventory-scan", Status: "skipped", Detail: "read-only; rerun without --dry-run to apply"})
	} else {
		_, scanErr := inventory.Scan(ctx.DB, ctx.Project.RepoRoot, scanPaths, true, "housekeeping")
		step = housekeepingStep{Name: "inventory-scan"}
		if scanErr != nil {
			step.Status = "failed"
			step.Detail = scanErr.Error()
		} else {
			step.Status = "ran"
			step.Detail = "inventory refreshed"
		}
		res.Steps = append(res.Steps, step)
	}

	// Step 4: optional verify query. Runs only when both flags are set,
	// since verification is project-specific (no sensible default).
	if verifyCommand != "" && verifyScope != "" {
		step = housekeepingStep{Name: "verify-query"}
		vRes := verification.Query(ctx.DB, verifyCommand, verifyScope, nil, true)
		step.Status = "ran"
		step.Detail = verification.CompactQueryLine(vRes)
		res.Steps = append(res.Steps, step)
	} else if verifyCommand != "" || verifyScope != "" {
		res.Steps = append(res.Steps, housekeepingStep{Name: "verify-query", Status: "skipped", Detail: "pass both --verify-command and --verify-scope"})
	}

	return res
}

func cmdLedgerHousekeeping(open opener) *cobra.Command {
	var dryRun bool
	var verifyCommand, verifyScope string
	var scanPaths []string
	c := &cobra.Command{
		Use:   "housekeeping",
		Short: "Bundled hygiene: archive run --yes, plan reconcile --apply, inventory scan, optional verify query",
		Long: `Bundles the recurring hygiene steps into one command so weekly cron
entries and end-of-session agents do not hand-craft the sequence.

Steps (in order):
  1. archive run     — archive closed/resolved rows (skipped under --dry-run)
  2. plan reconcile  — detect and repair plan-tree drift
  3. inventory scan  — refresh repo_files (skipped under --dry-run)
  4. verify query    — only when both --verify-command and --verify-scope are set

Cron-safe: non-interactive, --json machine-readable, --dry-run previews.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			res := runHousekeepingSteps(ctx, dryRun, verifyCommand, verifyScope, scanPaths)
			if ctx.Out.JSON {
				return ctx.Out.PrintData(res)
			}
			for _, s := range res.Steps {
				marker := "ok"
				switch s.Status {
				case "failed":
					marker = "FAIL"
				case "would-run":
					marker = "preview"
				case "skipped":
					marker = "skip"
				}
				ctx.Out.Hint(fmt.Sprintf("[%s] %s: %s", marker, s.Name, s.Detail))
			}
			return nil
		},
	}
	c.Flags().BoolVar(&dryRun, "dry-run", false, "preview without writing")
	c.Flags().StringVar(&verifyCommand, "verify-command", "", "verification command string (optional; runs verify query when set)")
	c.Flags().StringVar(&verifyScope, "verify-scope", "", "verification scope (used with --verify-command)")
	c.Flags().StringSliceVar(&scanPaths, "scan-paths", nil, "limit inventory scan to specific paths (default: whole repo)")
	return c
}
