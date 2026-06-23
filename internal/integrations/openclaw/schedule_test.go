package openclaw

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

type fakeCommandRunner struct {
	jobs  map[string]string
	calls [][]string
}

func (f *fakeCommandRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	call := append([]string{name}, args...)
	f.calls = append(f.calls, call)
	if len(args) >= 3 && args[0] == "cron" && args[1] == "list" {
		jobs := make([]map[string]string, 0, len(f.jobs))
		for jobName, id := range f.jobs {
			jobs = append(jobs, map[string]string{"id": id, "name": jobName})
		}
		return json.Marshal(map[string]any{"jobs": jobs})
	}
	return []byte(`{"ok":true}`), nil
}

func testScheduleConfig(t *testing.T) Config {
	t.Helper()
	return Config{Workspace: t.TempDir(), Repo: t.TempDir()}
}

func TestSchedulePreviewContainsExactCommandsWithoutMutation(t *testing.T) {
	runner := &fakeCommandRunner{jobs: map[string]string{}}
	operations, err := PlanSchedule(context.Background(), runner, testScheduleConfig(t), "/path with spaces/devdb-openclaw")
	if err != nil {
		t.Fatal(err)
	}
	if len(operations) != 2 || len(runner.calls) != 1 {
		t.Fatalf("operations=%+v calls=%+v", operations, runner.calls)
	}
	for _, operation := range operations {
		if operation.Action != "create" || !strings.Contains(operation.Command, "cron") || !strings.Contains(operation.Command, "add") || !strings.Contains(operation.Command, "path with spaces") {
			t.Fatalf("operation=%+v", operation)
		}
	}
}

func TestApplyScheduleCreatesMissingJobs(t *testing.T) {
	runner := &fakeCommandRunner{jobs: map[string]string{}}
	operations, err := PlanSchedule(context.Background(), runner, testScheduleConfig(t), "devdb-openclaw")
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplySchedule(context.Background(), runner, operations); err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 3 {
		t.Fatalf("calls=%+v", runner.calls)
	}
}

func TestExistingSchedulesAreUpdatedNotDuplicated(t *testing.T) {
	config := testScheduleConfig(t)
	suffix := strings.TrimPrefix(workspaceNamespace(config.Workspace), "openclaw-")
	runner := &fakeCommandRunner{jobs: map[string]string{
		"devdb-openclaw-sync-" + suffix:     "sync-id",
		"devdb-openclaw-friction-" + suffix: "friction-id",
	}}
	operations, err := PlanSchedule(context.Background(), runner, config, "devdb-openclaw")
	if err != nil {
		t.Fatal(err)
	}
	for _, operation := range operations {
		if operation.Action != "update" || operation.JobID == "" || operation.Args[1] != "edit" {
			t.Fatalf("operation=%+v", operation)
		}
		if strings.Contains(strings.Join(operation.Args, " "), " cron add ") {
			t.Fatal(fmt.Sprintf("existing job would be duplicated: %+v", operation))
		}
	}
}
