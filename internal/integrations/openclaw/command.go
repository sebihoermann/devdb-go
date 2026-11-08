package openclaw

import (
	"fmt"
	"os"
	"strings"

	"github.com/sebihoermann/devdb-go/internal/app"
	"github.com/spf13/cobra"
)

type commandOptions struct {
	workspace string
	repo      string
	json      bool
	// modelID stamps rows this adapter writes. Empty means "fall back to
	// DEVDB_MODEL_ID". It is a flag as well as an env var because the caller
	// (a gateway) knows the live model per turn and its process environment
	// does not change when the model does.
	modelID string
}

type runtime struct {
	config Config
	app    *app.Context
}

func (r *runtime) close() {
	if r != nil && r.app != nil {
		_ = r.app.Close()
	}
}

func openRuntime(opts *commandOptions, requireDB bool) (*runtime, error) {
	config, err := ResolveConfig(opts.workspace, opts.repo, opts.json)
	if err != nil {
		return nil, err
	}
	ctx, err := app.Open(config.Repo, "", config.JSON)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(opts.modelID) != "" {
		ctx.ModelID = strings.TrimSpace(opts.modelID)
	}
	if requireDB {
		if err := ctx.RequireDB(); err != nil {
			_ = ctx.Close()
			return nil, err
		}
	}
	return &runtime{config: config, app: ctx}, nil
}

// NewCommand builds the independently installed OpenClaw adapter CLI.
func NewCommand() *cobra.Command {
	opts := commandOptions{}
	root := &cobra.Command{
		Use:           "devdb-openclaw",
		Short:         "Index OpenClaw memory in a devdb ledger",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	root.PersistentFlags().StringVar(&opts.workspace, "workspace", "", "OpenClaw workspace root")
	root.PersistentFlags().StringVar(&opts.repo, "repo", "", "target repository containing .devdb")
	root.PersistentFlags().BoolVar(&opts.json, "json", false, "machine-readable JSON output")
	root.PersistentFlags().StringVar(&opts.modelID, "model-id", "", "model id recorded on rows this run writes (default: $DEVDB_MODEL_ID)")
	root.AddCommand(newVersionCommand(), newListCommand(&opts), newLinksCommand(&opts), newSyncCommand(&opts), newFrictionCommand(&opts), newDoctorCommand(&opts))
	return root
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print adapter version information",
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintln(cmd.OutOrStdout(), "devdb-openclaw dev")
		},
	}
}

func pendingCommand(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fmt.Errorf("%s is not implemented", use)
		},
	}
}

// Execute runs the adapter and returns a process exit code.
func Execute() int {
	cmd := NewCommand()
	cmd.SetOut(os.Stdout)
	cmd.SetErr(os.Stderr)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	return 0
}
