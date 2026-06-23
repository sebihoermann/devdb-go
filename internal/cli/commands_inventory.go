package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/sebihoermann/devdb-go/internal/domain/architecture"
	"github.com/sebihoermann/devdb-go/internal/domain/grasscutter"
	"github.com/sebihoermann/devdb-go/internal/domain/inventory"
	"github.com/sebihoermann/devdb-go/internal/git"
	"github.com/sebihoermann/devdb-go/internal/output"
	"github.com/spf13/cobra"
)

func cmdInventory(open opener) *cobra.Command {
	inv := &cobra.Command{Use: "inventory", Short: "File inventory and context reads"}
	inv.AddCommand(cmdInventoryScan(open), cmdInventoryLoc(open), cmdInventoryContext(open), cmdInventoryDiff(open), cmdInventorySuggestCuts(open))
	return inv
}

func cmdInventoryScan(open opener) *cobra.Command {
	var paths []string
	var gitAware, dryRun bool
	c := &cobra.Command{
		Use:   "scan",
		Short: "Scan repository files and update inventory",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			if dryRun {
				records, err := inventory.ScanInventory(ctx.Project.RepoRoot, paths, gitAware)
				if err != nil {
					return err
				}
				if ctx.Out.JSON {
					return ctx.Out.PrintData(map[string]any{"dry_run": true, "files_seen": len(records)})
				}
				return ctx.Out.PrintData(output.HumanLines{Lines: []string{
					fmt.Sprintf("dry-run: would scan %d files", len(records)),
				}})
			}
			res, err := inventory.Scan(ctx.DB, ctx.Project.RepoRoot, paths, gitAware, ctx.ModelID)
			if err != nil {
				return err
			}
			ctx.Out.Hint("scan %s · seen %d · +%d ~%d -%d",
				res.RunID[:8], res.FilesSeen, res.FilesAdded, res.FilesChanged, res.FilesRemoved)
			return ctx.Out.WriteResult(res.RunID, map[string]any{
				"kind": "scan_run", "files_seen": res.FilesSeen,
				"files_added": res.FilesAdded, "files_changed": res.FilesChanged, "files_removed": res.FilesRemoved,
			})
		},
	}
	c.Flags().StringSliceVar(&paths, "paths", nil, "paths to scan (default: entire repo)")
	c.Flags().BoolVar(&gitAware, "git-aware", false, "use git ls-files (respects .gitignore)")
	c.Flags().BoolVar(&dryRun, "dry-run", false, "discover files without writing")
	return c
}

func cmdInventoryLoc(open opener) *cobra.Command {
	var paths []string
	var gitAware bool
	c := &cobra.Command{
		Use:   "loc",
		Short: "Line-count summary from current files",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			sum, err := inventory.Loc(ctx.Project.RepoRoot, paths, gitAware)
			if err != nil {
				return err
			}
			if ctx.Out.JSON {
				return ctx.Out.PrintData(sum)
			}
			lines := []string{
				fmt.Sprintf("files: %d · lines: %d", sum.Files, sum.TotalLines),
			}
			for kind, n := range sum.ByKind {
				lines = append(lines, fmt.Sprintf("  %s: %d", kind, n))
			}
			return ctx.Out.PrintData(output.HumanLines{Lines: lines})
		},
	}
	c.Flags().StringSliceVar(&paths, "paths", nil, "paths to scan")
	c.Flags().BoolVar(&gitAware, "git-aware", false, "use git ls-files")
	return c
}

func cmdInventoryContext(open opener) *cobra.Command {
	var files []string
	var task, planItem string
	var strict bool
	c := &cobra.Command{
		Use:   "context",
		Short: "Architecture notes and findings for files you will touch",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			payload, err := inventory.Context(ctx.DB, inventory.ContextOptions{
				Files: files, Task: task, PlanItemID: planItem,
			})
			if err != nil {
				return err
			}
			if ctx.Out.JSON {
				if err := ctx.Out.PrintData(payload); err != nil {
					return err
				}
			} else if err := ctx.Out.PrintData(output.HumanLines{Lines: inventory.FormatContextHuman(payload)}); err != nil {
				return err
			}
			if strict && inventory.ContextStrictExit(payload) {
				return &CLIError{Code: ExitUsage, Message: "strict context: stale notes or high-severity findings", Kind: "strict_context"}
			}
			return nil
		},
	}
	c.Flags().StringSliceVar(&files, "files", nil, "file paths to inspect")
	c.Flags().StringVar(&task, "task", "", "optional task description")
	c.Flags().StringVar(&planItem, "plan-item", "", "include plan-linked reminders")
	c.Flags().BoolVar(&strict, "strict", false, "exit non-zero on stale notes or high findings")
	return c
}

func cmdInventorySuggestCuts(open opener) *cobra.Command {
	var paths, principles []string
	var dryRun bool
	c := &cobra.Command{
		Use:   "suggest-cuts",
		Short: "Run grass-cutter heuristics to surface dead code and sprawl",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			var n int
			if err := ctx.DB.QueryRow(`SELECT COUNT(*) FROM repo_files`).Scan(&n); err != nil {
				return err
			}
			if n == 0 {
				return &CLIError{Code: ExitGeneral, Message: "no files in inventory (run inventory scan first)", Kind: "missing_inventory"}
			}
			gitSHA := git.HeadSHA(ctx.Project.RepoRoot)
			res, err := grasscutter.Run(ctx.DB, ctx.Project.RepoRoot, paths, principles, dryRun, gitSHA, ctx.ModelID)
			if err != nil {
				return err
			}
			if dryRun {
				if ctx.Out.JSON {
					return ctx.Out.PrintData(map[string]any{
						"persisted": false, "run_id": nil,
						"candidate_count": len(res.Candidates),
						"candidates":      res.Candidates,
						"counts":          res.Counts,
						"summary":         res.Summary,
					})
				}
				var lines []string
				for _, cand := range res.Candidates {
					loc := cand.FilePath
					if loc == "" {
						loc = "cross-cutting"
					}
					if cand.LineStart != nil {
						loc = fmt.Sprintf("%s:%d", loc, *cand.LineStart)
					}
					lines = append(lines, fmt.Sprintf("%s %s %s", cand.Principle, loc, cand.Title))
				}
				if err := ctx.Out.PrintData(output.HumanLines{Lines: lines}); err != nil {
					return err
				}
				ctx.Out.Hint("%s (preview only; no review run or findings written)", res.Summary)
				return nil
			}
			ctx.Out.Hint(res.Summary)
			return ctx.Out.WriteResult(res.RunID, map[string]any{
				"kind": "review_run", "tier": "grass-cutter",
				"candidate_count": len(res.Candidates), "persisted_count": res.PersistedN,
			})
		},
	}
	c.Flags().StringSliceVar(&paths, "paths", nil, "scope paths (default: entire repo)")
	c.Flags().StringSliceVar(&principles, "principles", nil, "dead|inlinable|sprawl|duplication|staleness")
	c.Flags().BoolVar(&dryRun, "dry-run", false, "preview candidates without opening a review run")
	return c
}

func cmdInventoryDiff(open opener) *cobra.Command {
	return &cobra.Command{
		Use:   "diff REF",
		Short: "Files changed since git ref with linked notes/findings",
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
			rows, err := inventory.DiffSince(ctx.DB, ctx.Project.RepoRoot, args[0])
			if err != nil {
				return &CLIError{Code: ExitGeneral, Message: err.Error(), Kind: "git_diff"}
			}
			if ctx.Out.JSON {
				return ctx.Out.PrintData(rows)
			}
			if len(rows) == 0 {
				return ctx.Out.PrintData(fmt.Sprintf("no changes since %s", args[0]))
			}
			var lines []string
			for _, row := range rows {
				lines = append(lines, row.Path)
				if len(row.ArchitectureNotes) > 0 {
					lines = append(lines, "  arch: "+strings.Join(row.ArchitectureNotes, ", "))
				}
				if len(row.OpenFindings) > 0 {
					lines = append(lines, "  findings: "+strings.Join(row.OpenFindings, ", "))
				}
			}
			return ctx.Out.PrintData(output.HumanLines{Lines: lines})
		},
	}
}

func cmdArch(open opener) *cobra.Command {
	arch := &cobra.Command{Use: "arch", Short: "Architecture notes"}
	arch.AddCommand(cmdArchAdd(open), cmdArchList(open), cmdArchShow(open),
		cmdArchUpdate(open), cmdArchVerify(open), cmdArchRender(open))
	return arch
}

func cmdArchAdd(open opener) *cobra.Command {
	var body, confidence string
	var sources []string
	c := &cobra.Command{
		Use:   "add TOPIC",
		Short: "Add an architecture note",
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
			if body == "" {
				return usageError("--body is required")
			}
			if len(sources) == 0 {
				return usageError("at least one --source is required")
			}
			id, err := architecture.Add(ctx.DB, args[0], body, sources, confidence, ctx.ModelID)
			if err != nil {
				if errors.Is(err, architecture.ErrInvalidTopic) {
					return &CLIError{Code: ExitInvalidValue, Message: err.Error(), Kind: "invalid_topic"}
				}
				var missingSource *architecture.MissingSourceError
				if errors.As(err, &missingSource) {
					return &CLIError{Code: ExitNotFound, Message: err.Error(), Kind: "missing_source"}
				}
				return err
			}
			return ctx.Out.WriteResult(id, map[string]any{"kind": "architecture_note"})
		},
	}
	c.Flags().StringVar(&body, "body", "", "note body (2-5 sentences)")
	c.Flags().StringSliceVar(&sources, "source", nil, "source file path (repeatable)")
	c.Flags().StringVar(&confidence, "confidence", "medium", "high|medium|low")
	return c
}

func cmdArchList(open opener) *cobra.Command {
	var touching, status string
	var stale bool
	c := &cobra.Command{
		Use:   "list",
		Short: "List architecture notes",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			notes, err := architecture.List(ctx.DB, architecture.ListFilter{
				TouchingPath: touching, Status: status, Stale: stale,
			})
			if err != nil {
				return err
			}
			return ctx.Out.PrintData(notes)
		},
	}
	c.Flags().StringVar(&touching, "touching", "", "filter by source file path")
	c.Flags().StringVar(&status, "status", "", "filter by status")
	c.Flags().BoolVar(&stale, "stale", false, "only stale notes")
	return c
}

func cmdArchShow(open opener) *cobra.Command {
	return &cobra.Command{
		Use:   "show ID",
		Short: "Show one architecture note",
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
			note, err := architecture.Get(ctx.DB, args[0])
			if err != nil {
				return err
			}
			if note == nil {
				return notFoundError("note not found")
			}
			return ctx.Out.PrintData(note)
		},
	}
}

func cmdArchUpdate(open opener) *cobra.Command {
	var body, confidence string
	var sources []string
	c := &cobra.Command{
		Use:   "update ID",
		Short: "Update an architecture note",
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
			var bodyPtr *string
			if body != "" {
				bodyPtr = &body
			}
			var confPtr *string
			if confidence != "" {
				confPtr = &confidence
			}
			var srcSlice []string
			if len(sources) > 0 {
				srcSlice = sources
			}
			id, ok, err := architecture.Update(ctx.DB, args[0], bodyPtr, srcSlice, confPtr)
			if err != nil {
				var missingSource *architecture.MissingSourceError
				if errors.As(err, &missingSource) {
					return &CLIError{Code: ExitNotFound, Message: err.Error(), Kind: "missing_source"}
				}
				return err
			}
			if !ok {
				return notFoundError("note not found")
			}
			return ctx.Out.WriteResult(id, map[string]any{"kind": "architecture_note", "action": "update"})
		},
	}
	c.Flags().StringVar(&body, "body", "", "new body")
	c.Flags().StringSliceVar(&sources, "source", nil, "new source paths")
	c.Flags().StringVar(&confidence, "confidence", "", "new confidence")
	return c
}

func cmdArchVerify(open opener) *cobra.Command {
	return &cobra.Command{
		Use:   "verify [ID|all]",
		Short: "Verify note sources still match",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			bulk := flagAll || (len(args) == 1 && args[0] == "all")
			if bulk {
				if len(args) == 1 && args[0] != "all" {
					return usageError("cannot combine note id with --all")
				}
				res, err := architecture.VerifyAll(ctx.DB)
				if err != nil {
					return err
				}
				if ctx.Out.JSON {
					return ctx.Out.PrintData(res)
				}
				return ctx.Out.PrintData(fmt.Sprintf("verified %d notes, %d stale", res.Verified, res.Stale))
			}
			if len(args) != 1 {
				return usageError("note id, 'all', or --all required")
			}
			status, _, id, err := architecture.Verify(ctx.DB, args[0])
			if err != nil {
				if errors.Is(err, architecture.ErrNoteNotFound) {
					return notFoundError(err.Error())
				}
				return err
			}
			if ctx.Out.JSON {
				return ctx.Out.PrintData(map[string]any{"id": id, "status": status})
			}
			return ctx.Out.PrintData(status)
		},
	}
}

func cmdArchRender(open opener) *cobra.Command {
	var outPath string
	c := &cobra.Command{
		Use:   "render",
		Short: "Render architecture notes to markdown",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			md, err := architecture.RenderMarkdown(ctx.DB)
			if err != nil {
				return err
			}
			if outPath != "" {
				if err := os.WriteFile(outPath, []byte(md), 0o644); err != nil {
					return err
				}
				ctx.Out.Hint("wrote %s", outPath)
				if ctx.Out.JSON {
					return ctx.Out.PrintData(map[string]any{"path": outPath, "bytes": len(md)})
				}
				return ctx.Out.PrintData(outPath)
			}
			return ctx.Out.PrintData(md)
		},
	}
	c.Flags().StringVar(&outPath, "output", "", "write markdown to file")
	return c
}
