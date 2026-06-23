package openclaw

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// CommandRunner isolates public OpenClaw CLI calls for deterministic tests.
type CommandRunner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}

type execCommandRunner struct{}

func (execCommandRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

// ScheduleOperation is one exact OpenClaw CLI mutation.
type ScheduleOperation struct {
	Action  string   `json:"action"`
	Name    string   `json:"name"`
	JobID   string   `json:"job_id,omitempty"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

type cronList struct {
	Jobs []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"jobs"`
}

type scheduleSpec struct {
	name, expression, message string
}

// PlanSchedule reads current jobs and returns exact create/update operations.
func PlanSchedule(ctx context.Context, runner CommandRunner, config Config, binary string) ([]ScheduleOperation, error) {
	output, err := runner.Run(ctx, "openclaw", "cron", "list", "--all", "--json")
	if err != nil {
		return nil, fmt.Errorf("list OpenClaw cron jobs: %w: %s", err, strings.TrimSpace(string(output)))
	}
	var current cronList
	if err := json.Unmarshal(output, &current); err != nil {
		return nil, fmt.Errorf("parse OpenClaw cron list: %w", err)
	}
	existing := map[string]string{}
	for _, job := range current.Jobs {
		existing[job.Name] = job.ID
	}

	if strings.TrimSpace(binary) == "" {
		binary = "devdb-openclaw"
	}
	suffix := strings.TrimPrefix(workspaceNamespace(config.Workspace), "openclaw-")
	base := shellCommand(binary, config)
	specs := []scheduleSpec{
		{name: "devdb-openclaw-sync-" + suffix, expression: "17 3 * * *", message: "Execute this command and report only failures: " + base + " sync"},
		{name: "devdb-openclaw-friction-" + suffix, expression: "13 6 * * *", message: "Execute this command and report only failures: " + base + " friction scan"},
	}
	operations := make([]ScheduleOperation, 0, len(specs))
	for _, spec := range specs {
		description := "managed by devdb-openclaw"
		if id := existing[spec.name]; id != "" {
			args := []string{"cron", "edit", id, "--name", spec.name, "--description", description, "--cron", spec.expression, "--session", "isolated", "--message", spec.message, "--no-deliver"}
			operations = append(operations, ScheduleOperation{Action: "update", Name: spec.name, JobID: id, Command: formatCommand("openclaw", args), Args: args})
			continue
		}
		args := []string{"cron", "add", "--name", spec.name, "--description", description, "--cron", spec.expression, "--session", "isolated", "--message", spec.message, "--no-deliver", "--json"}
		operations = append(operations, ScheduleOperation{Action: "create", Name: spec.name, Command: formatCommand("openclaw", args), Args: args})
	}
	return operations, nil
}

// ApplySchedule executes an already-previewed operation list.
func ApplySchedule(ctx context.Context, runner CommandRunner, operations []ScheduleOperation) error {
	for _, operation := range operations {
		output, err := runner.Run(ctx, "openclaw", operation.Args...)
		if err != nil {
			return fmt.Errorf("%s schedule %s: %w: %s", operation.Action, operation.Name, err, strings.TrimSpace(string(output)))
		}
	}
	return nil
}

func shellCommand(binary string, config Config) string {
	return strings.Join([]string{strconv.Quote(binary), "--workspace", strconv.Quote(config.Workspace), "--repo", strconv.Quote(config.Repo)}, " ")
}

func formatCommand(name string, args []string) string {
	parts := []string{strconv.Quote(name)}
	for _, arg := range args {
		parts = append(parts, strconv.Quote(arg))
	}
	return strings.Join(parts, " ")
}

func newScheduleCommand(opts *commandOptions) *cobra.Command {
	var apply bool
	var binary string
	cmd := &cobra.Command{
		Use:   "schedule",
		Short: "Preview or apply OpenClaw scheduler entries",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			config, err := ResolveConfig(opts.workspace, opts.repo, opts.json)
			if err != nil {
				return err
			}
			runner := execCommandRunner{}
			operations, err := PlanSchedule(cmd.Context(), runner, config, binary)
			if err != nil {
				return err
			}
			if config.JSON {
				if err := json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{"apply": apply, "operations": operations}); err != nil {
					return err
				}
			} else {
				mode := "preview"
				if apply {
					mode = "apply"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "mode: %s\n", mode)
				for _, operation := range operations {
					fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n  $ %s\n", operation.Action, operation.Name, operation.Command)
				}
			}
			if !apply {
				return nil
			}
			return ApplySchedule(cmd.Context(), runner, operations)
		},
	}
	cmd.Flags().BoolVar(&apply, "apply", false, "apply the previewed OpenClaw cron changes")
	cmd.Flags().StringVar(&binary, "binary", "devdb-openclaw", "adapter command used in scheduled agent messages")
	return cmd
}
