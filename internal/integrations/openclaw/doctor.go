package openclaw

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/sebihoermann/devdb-go/internal/app"
	"github.com/spf13/cobra"
)

// DoctorCheck is one adapter readiness signal.
type DoctorCheck struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

// DoctorResult aggregates adapter readiness without mutating state.
type DoctorResult struct {
	Ready  bool          `json:"ready"`
	Checks []DoctorCheck `json:"checks"`
}

// Doctor inspects workspace, ledger, schema, and OpenClaw CLI readiness.
func Doctor(config Config, lookPath func(string) (string, error)) DoctorResult {
	result := DoctorResult{Ready: true}
	add := func(name string, err error, detail string) {
		check := DoctorCheck{Name: name, OK: err == nil, Detail: detail}
		if err != nil {
			check.Detail = err.Error()
			result.Ready = false
		}
		result.Checks = append(result.Checks, check)
	}

	info, err := os.Stat(config.Workspace)
	if err == nil && !info.IsDir() {
		err = fmt.Errorf("workspace is not a directory")
	}
	add("workspace-readable", err, config.Workspace)

	dbPath := filepath.Join(config.Repo, ".devdb", "development.db")
	_, err = os.Stat(dbPath)
	add("target-ledger", err, dbPath)

	ctx, openErr := app.Open(config.Repo, "", false)
	if openErr == nil {
		defer ctx.Close()
		openErr = ctx.RequireDB()
	}
	add("devdb-compatible", openErr, "Go-native devdb schema")

	cliPath, err := lookPath("openclaw")
	add("openclaw-cli", err, cliPath)
	return result
}

func newDoctorCommand(opts *commandOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check adapter readiness",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			config, err := ResolveConfig(opts.workspace, opts.repo, opts.json)
			if err != nil {
				return err
			}
			result := Doctor(config, exec.LookPath)
			if config.JSON {
				if err := json.NewEncoder(cmd.OutOrStdout()).Encode(result); err != nil {
					return err
				}
			} else {
				for _, check := range result.Checks {
					status := "ok"
					if !check.OK {
						status = "fail"
					}
					fmt.Fprintf(cmd.OutOrStdout(), "%-18s %-4s %s\n", check.Name, status, check.Detail)
				}
			}
			if !result.Ready {
				return fmt.Errorf("adapter is not ready")
			}
			return nil
		},
	}
}
