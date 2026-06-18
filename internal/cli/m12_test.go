package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestM12GlobalAllExpandsFeedbackList(t *testing.T) {
	bin := buildDevdb(t)
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "development.db")
	runDevdb(t, bin, "--db", dbPath, "init")

	for i := 0; i < 25; i++ {
		_, _, code := runDevdb(t, bin, "--db", dbPath, "feedback", "add", "--role", "model", "note", strings.Repeat("x", i))
		if code != 0 {
			t.Fatalf("feedback add %d failed", i)
		}
	}

	capped, _, code := runDevdb(t, bin, "--db", dbPath, "--json", "feedback", "list")
	if code != 0 {
		t.Fatal("feedback list failed")
	}
	var cappedRows []map[string]any
	if err := json.Unmarshal([]byte(capped), &cappedRows); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(cappedRows) != 20 {
		t.Fatalf("default cap: got %d rows, want 20", len(cappedRows))
	}

	all, _, code := runDevdb(t, bin, "--db", dbPath, "--json", "--all", "feedback", "list")
	if code != 0 {
		t.Fatal("feedback list --all failed")
	}
	var allRows []map[string]any
	if err := json.Unmarshal([]byte(all), &allRows); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(allRows) != 25 {
		t.Fatalf("--all: got %d rows, want 25", len(allRows))
	}
}

func TestM12ArchVerifyAll(t *testing.T) {
	bin := buildDevdb(t)
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	dbPath := filepath.Join(dir, "development.db")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	runDevdb(t, bin, "--repo", repo, "--db", dbPath, "init")
	runDevdb(t, bin, "--repo", repo, "--db", dbPath, "inventory", "scan", "--paths", ".")

	stdout, _, code := runDevdb(t, bin, "--repo", repo, "--db", dbPath, "--json", "arch", "verify", "all")
	if code != 0 {
		t.Fatalf("arch verify all exit %d", code)
	}
	var res map[string]any
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatalf("json: %v stdout=%q", err, stdout)
	}
	if _, ok := res["verified"]; !ok {
		t.Fatalf("missing verified key: %v", res)
	}
	if _, ok := res["stale"]; !ok {
		t.Fatalf("missing stale key: %v", res)
	}

	flagOut, _, code := runDevdb(t, bin, "--repo", repo, "--db", dbPath, "--json", "--all", "arch", "verify")
	if code != 0 {
		t.Fatalf("arch verify --all exit %d", code)
	}
	if err := json.Unmarshal([]byte(flagOut), &res); err != nil {
		t.Fatalf("json: %v", err)
	}
}

func TestM12ReminderListAll(t *testing.T) {
	bin := buildDevdb(t)
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "development.db")
	runDevdb(t, bin, "--db", dbPath, "init")

	for i := 0; i < 35; i++ {
		_, _, code := runDevdb(t, bin, "--db", dbPath, "reminder", "add", "r", "--due", "2026-12-31T00:00:00Z")
		if code != 0 {
			t.Fatalf("reminder add %d failed", i)
		}
	}

	capped, _, code := runDevdb(t, bin, "--db", dbPath, "--json", "reminder", "list", "--status", "all")
	if code != 0 {
		t.Fatal("reminder list failed")
	}
	var cappedRows []map[string]any
	if err := json.Unmarshal([]byte(capped), &cappedRows); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(cappedRows) != 30 {
		t.Fatalf("default cap: got %d rows, want 30", len(cappedRows))
	}

	all, _, code := runDevdb(t, bin, "--db", dbPath, "--json", "--all", "reminder", "list", "--status", "all")
	if code != 0 {
		t.Fatal("reminder list --all failed")
	}
	var allRows []map[string]any
	if err := json.Unmarshal([]byte(all), &allRows); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(allRows) != 35 {
		t.Fatalf("--all: got %d rows, want 35", len(allRows))
	}
}

func TestM12ReportAllExpandsFeedback(t *testing.T) {
	bin := buildDevdb(t)
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "development.db")
	runDevdb(t, bin, "--db", dbPath, "init")

	for i := 0; i < 8; i++ {
		_, _, code := runDevdb(t, bin, "--db", dbPath, "feedback", "add", "--role", "user", "note")
		if code != 0 {
			t.Fatalf("feedback add %d failed", i)
		}
	}

	capped, _, code := runDevdb(t, bin, "--db", dbPath, "--json", "report")
	if code != 0 {
		t.Fatal("report failed")
	}
	var cappedReport map[string]any
	if err := json.Unmarshal([]byte(capped), &cappedReport); err != nil {
		t.Fatalf("json: %v", err)
	}
	cappedFB, _ := cappedReport["feedback"].([]any)
	if len(cappedFB) != 5 {
		t.Fatalf("report default feedback cap: got %d want 5", len(cappedFB))
	}

	all, _, code := runDevdb(t, bin, "--db", dbPath, "--json", "--all", "report")
	if code != 0 {
		t.Fatal("report --all failed")
	}
	var allReport map[string]any
	if err := json.Unmarshal([]byte(all), &allReport); err != nil {
		t.Fatalf("json: %v", err)
	}
	allFB, _ := allReport["feedback"].([]any)
	if len(allFB) != 8 {
		t.Fatalf("report --all feedback: got %d want 8", len(allFB))
	}
}

func TestM12JSONContractReads(t *testing.T) {
	bin := buildDevdb(t)
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	dbPath := filepath.Join(dir, "development.db")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	setup := []string{"--repo", repo, "--db", dbPath}
	jsonBase := append(setup, "--json")

	runDevdb(t, bin, append(setup, "init")...)
	planID := firstLine(runDevdbStdout(t, bin, append(setup, "plan", "create", "JSON plan", "--slug", "json-plan")...))
	msID := firstLine(runDevdbStdout(t, bin, append(setup, "plan", "milestone", "add", "M1", "--plan", planID)...))
	itemID := firstLine(runDevdbStdout(t, bin, append(setup, "plan", "item", "add", "Item", "--plan", planID, "--milestone", msID)...))
	_ = firstLine(runDevdbStdout(t, bin, append(setup, "feedback", "add", "--role", "user", "hello")...))
	_ = firstLine(runDevdbStdout(t, bin, append(setup, "goal", "add", "Ship", "--kind", "goal")...))
	_ = firstLine(runDevdbStdout(t, bin, append(setup, "task", "add", "Task one")...))
	_ = firstLine(runDevdbStdout(t, bin, append(setup, "reminder", "add", "Ping", "--due", "2026-12-31T00:00:00Z")...))

	cases := []struct {
		name string
		args []string
		keys []string
	}{
		{"status", []string{"status"}, []string{"overall", "open_plan_items", "in_progress_plan_items"}},
		{"quality", []string{"quality"}, []string{"open_high_feedback", "open_findings"}},
		{"report", []string{"report"}, []string{"status", "quality", "feedback"}},
		{"resume", []string{"resume"}, []string{"in_flight"}},
		{"feedback_list", []string{"feedback", "list"}, nil},
		{"goal_list", []string{"goal", "list"}, nil},
		{"task_list", []string{"task", "list"}, nil},
		{"reminder_list", []string{"reminder", "list"}, nil},
		{"plan_list", []string{"plan", "list"}, nil},
		{"plan_item_show", []string{"plan", "item", "show", itemID}, []string{"item", "acceptance"}},
		{"arch_list", []string{"arch", "list"}, nil},
		{"review_list", []string{"review", "list"}, nil},
		{"analytics_summary", []string{"analytics", "summary"}, []string{"total", "top_failure_kinds"}},
		{"doctor", []string{"doctor"}, []string{"schema_kind", "repo_root"}},
		{"inventory_loc", []string{"inventory", "loc"}, []string{"files", "total_lines"}},
		{"verify_query", []string{"verify", "query", "--command", "go test ./...", "--scope", "."}, []string{"decision", "fresh"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stdout, _, code := runDevdb(t, bin, append(jsonBase, tc.args...)...)
			if code != 0 {
				t.Fatalf("exit %d stdout=%q", code, stdout)
			}
			var payload any
			if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
				t.Fatalf("invalid JSON: %v\n%s", err, stdout)
			}
			if tc.keys == nil {
				return
			}
			obj, ok := payload.(map[string]any)
			if !ok {
				t.Fatalf("expected object, got %T", payload)
			}
			for _, k := range tc.keys {
				if _, ok := obj[k]; !ok {
					t.Fatalf("missing key %q in %v", k, obj)
				}
			}
		})
	}
}

func runDevdbStdout(t *testing.T, bin string, args ...string) string {
	t.Helper()
	stdout, _, code := runDevdb(t, bin, args...)
	if code != 0 {
		t.Fatalf("%v exit %d", args, code)
	}
	return stdout
}
