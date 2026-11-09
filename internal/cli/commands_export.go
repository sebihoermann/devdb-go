package cli

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sebihoermann/devdb-go/internal/domain/export"
	"github.com/sebihoermann/devdb-go/internal/git"
	"github.com/sebihoermann/devdb-go/internal/storage"
	"github.com/spf13/cobra"
)

// cmdLedgerExport renders the ledger as a prose report for a human
// administrator.
//
// The report goes to stdout by default so it can be piped into a file the
// administrator decides to commit. --output is there for callers that would
// rather the tool did the writing.
//
// The document is deterministic for an unchanged ledger: it carries no
// wall-clock stamp unless --generated-at asks for one, and it renders recorded
// dates absolutely. A diff in a committed report therefore means the ledger
// really moved, not that someone regenerated it on a different day.
func cmdLedgerExport(open opener) *cobra.Command {
	var views []string
	var outPath string
	var generatedAt bool
	c := &cobra.Command{
		Use:   "export",
		Short: "Render the ledger as a prose report",
		Long: `Render the ledger as a prose report for a human administrator.

The report says what state the project is in, what is moving, what is stuck,
and what needs a decision. It is written to stdout so it can be piped into a
file and committed:

  devdb ledger export > docs/ledger.md

Use --view to narrow the document to the parts you care about, and the global
--all to lift the per-section row caps.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := open(cmd)
			if err != nil {
				return err
			}
			defer ctx.Close()
			if err := ctx.RequireDB(); err != nil {
				return err
			}
			resolved, err := export.ResolveViews(views)
			if err != nil {
				return &CLIError{Code: ExitUsage, Message: err.Error(), Kind: "invalid_argument"}
			}
			kind, ver, _ := storage.DetectSchema(ctx.DB)
			opts := export.Options{
				Views:         views,
				RepoRoot:      ctx.Project.RepoRoot,
				DBPath:        ctx.Project.DBPath,
				GitSHA:        git.HeadSHA(ctx.Project.RepoRoot),
				GitBranch:     git.Branch(ctx.Project.RepoRoot),
				SchemaKind:    string(kind),
				SchemaVersion: ver,
			}
			if flagAll {
				// A negative limit means "no cap": the domain layer reads a
				// limit of zero as "no LIMIT clause", but the export layer
				// reads zero as "use the per-view default".
				opts.Limit = -1
			}
			if generatedAt {
				opts.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
			}
			md, err := export.RenderMarkdown(ctx.DB, opts)
			if err != nil {
				return &CLIError{Code: ExitUsage, Message: err.Error(), Kind: "invalid_argument"}
			}
			if outPath == "" {
				return ctx.Out.PrintData(md)
			}
			if dir := filepath.Dir(outPath); dir != "" && dir != "." {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					return err
				}
			}
			if err := os.WriteFile(outPath, []byte(md), 0o644); err != nil {
				return err
			}
			ctx.Out.Hint("wrote %s", outPath)
			if ctx.Out.JSON {
				return ctx.Out.PrintData(map[string]any{
					"path":  outPath,
					"bytes": len(md),
					"views": resolved,
				})
			}
			return ctx.Out.PrintData(outPath)
		},
	}
	c.Flags().StringSliceVar(&views, "view", nil, "views to include: "+strings.Join(export.ViewNames(), ", "))
	c.Flags().StringVar(&outPath, "output", "", "write the report to PATH instead of stdout")
	c.Flags().BoolVar(&generatedAt, "generated-at", false, "stamp the current time into the report header")
	return c
}
