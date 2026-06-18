package cli

import (
	"fmt"

	"github.com/sebihoermann/devdb-go/internal/app"
	"github.com/sebihoermann/devdb-go/internal/domain/feedback"
	"github.com/sebihoermann/devdb-go/internal/output"
	"github.com/spf13/cobra"
)

func cmdFeedbackImport(open opener) *cobra.Command {
	importCmd := &cobra.Command{Use: "import", Short: "Bulk import feedback archives"}
	importCmd.AddCommand(cmdFeedbackImportMarkdown(open), cmdFeedbackImportCommits(open))
	return importCmd
}

func cmdFeedbackImportMarkdown(open opener) *cobra.Command {
	return &cobra.Command{
		Use:   "markdown PATH",
		Short: "Import feedback rows from a markdown file (one entry per ## heading)",
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
			result, err := feedback.ImportMarkdown(ctx.DB, args[0], ctx.ModelID)
			if err != nil {
				return err
			}
			return emitFeedbackImportResult(ctx, result.Imported, result.IDs, "imported", "feedback_import")
		},
	}
}

func cmdFeedbackImportCommits(open opener) *cobra.Command {
	var branches []string
	var limit int
	c := &cobra.Command{
		Use:   "commits",
		Short: "Import commit history into commit_archeology (legacy import-branch-commits)",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			if len(branches) == 0 {
				return usageError("--branches is required")
			}
			result, err := feedback.ImportCommits(ctx.DB, ctx.Project.RepoRoot, branches, limit, ctx.ModelID)
			if err != nil {
				return err
			}
			for _, w := range result.Warnings {
				ctx.Out.Hint("warning: %s", w)
			}
			return emitFeedbackImportResult(ctx, result.Inserted, result.IDs, "inserted", "commit_archeology")
		},
	}
	c.Flags().StringSliceVar(&branches, "branches", nil, "branch names to walk (required)")
	c.Flags().IntVar(&limit, "limit", 200, "max commits per branch")
	return c
}

func emitFeedbackImportResult(ctx *app.Context, count int, ids []string, verb, kind string) error {
	if ctx.Out.JSON {
		return ctx.Out.PrintData(map[string]any{
			verb: count,
			"ids": ids,
			"kind": kind,
		})
	}
	if count == 1 && len(ids) == 1 {
		ctx.Out.Hint("%s %d row", verb, count)
		return ctx.Out.WriteResult(ids[0], map[string]any{"kind": kind, "count": 1})
	}
	ctx.Out.Hint("%s %d row(s)", verb, count)
	return ctx.Out.PrintData(output.HumanLines{Lines: []string{fmt.Sprintf("%s %d", verb, count)}})
}
