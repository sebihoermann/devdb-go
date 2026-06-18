package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// goldenDir holds file-backed expected CLI output (M13).
var goldenDir = filepath.Join("testdata", "golden")

var (
	reFullID  = regexp.MustCompile(`\b[a-f0-9]{32}\b`)
	rePrefix  = regexp.MustCompile(`\b[a-f0-9]{8}\b`)
	reGitLine = regexp.MustCompile(`(?m)^git:.*\n?`)
)

func normalizeGolden(s string) string {
	s = reGitLine.ReplaceAllString(s, "")
	s = reFullID.ReplaceAllString(s, "<ID>")
	s = rePrefix.ReplaceAllString(s, "<PREFIX>")
	return strings.TrimSpace(s) + "\n"
}

func normalizeGoldenJSON(s string) string {
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return normalizeGolden(s)
	}
	v = normalizeJSONValue(v)
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return normalizeGolden(s)
	}
	return normalizeGolden(string(out))
}

func normalizeJSONValue(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			if isVolatileJSONField(k) {
				if val != nil {
					out[k] = "<TIMESTAMP>"
				}
				continue
			}
			out[k] = normalizeJSONValue(val)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			out[i] = normalizeJSONValue(val)
		}
		return out
	default:
		return v
	}
}

func isVolatileJSONField(key string) bool {
	switch key {
	case "created_at", "updated_at", "started_at", "finished_at", "last_verified_at",
		"due_at", "snooze_until", "dismissed_at", "archived_at", "committed_at", "last_seen_at":
		return true
	default:
		return false
	}
}

func assertGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join(goldenDir, name)
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("updated golden %s", path)
		return
	}
	wantBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (run UPDATE_GOLDEN=1 go test to create)", path, err)
	}
	want := string(wantBytes)
	if got != want {
		t.Fatalf("golden mismatch %s\n--- got ---\n%s--- want ---\n%s", name, got, want)
	}
}

type goldenSeed struct {
	repo   string
	dbPath string
	planID string
	msID   string
	itemID string
	accID  string
}

func seedGoldenDB(t *testing.T) goldenSeed {
	t.Helper()
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	dbPath := filepath.Join(dir, ".devdb", "development.db")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}

	base := []string{"--repo", repo, "--db", dbPath}
	run := func(args ...string) string {
		t.Helper()
		stdout, stderr, code := runCLI(t, append(base, args...)...)
		if code != 0 {
			t.Fatalf("%v exit %d stderr=%s stdout=%s", args, code, stderr, stdout)
		}
		return stdout
	}

	run("init")
	planID := firstLine(run("plan", "create", "Golden plan", "--slug", "golden"))
	msID := firstLine(run("plan", "milestone", "add", "M1", "--plan", planID))
	itemID := firstLine(run("plan", "item", "add", "Ship feature", "--plan", planID, "--milestone", msID))
	accID := firstLine(run("plan", "acceptance", "add", "works end-to-end", "--plan-item", itemID))
	_ = firstLine(run("feedback", "add", "--role", "model", "open feedback note", "--severity", "med"))
	_ = firstLine(run("goal", "add", "Ship M13", "--kind", "goal"))

	return goldenSeed{repo: repo, dbPath: dbPath, planID: planID, msID: msID, itemID: itemID, accID: accID}
}

func TestM13GoldenReads(t *testing.T) {
	seed := seedGoldenDB(t)
	base := []string{"--repo", seed.repo, "--db", seed.dbPath}

	t.Run("status_human", func(t *testing.T) {
		stdout, _, code := runCLI(t, append(base, "status")...)
		if code != 0 {
			t.Fatalf("exit %d", code)
		}
		assertGolden(t, "status.human.golden", normalizeGolden(stdout))
	})

	t.Run("status_json", func(t *testing.T) {
		stdout, _, code := runCLI(t, append(base, "--json", "status")...)
		if code != 0 {
			t.Fatalf("exit %d", code)
		}
		assertGolden(t, "status.json.golden", normalizeGoldenJSON(stdout))
	})

	t.Run("report_human", func(t *testing.T) {
		stdout, _, code := runCLI(t, append(base, "report")...)
		if code != 0 {
			t.Fatalf("exit %d", code)
		}
		assertGolden(t, "report.human.golden", normalizeGolden(stdout))
	})

	t.Run("report_json", func(t *testing.T) {
		stdout, _, code := runCLI(t, append(base, "--json", "report")...)
		if code != 0 {
			t.Fatalf("exit %d", code)
		}
		assertGolden(t, "report.json.golden", normalizeGoldenJSON(stdout))
	})

	t.Run("resume_empty", func(t *testing.T) {
		stdout, _, code := runCLI(t, append(base, "resume")...)
		if code != 0 {
			t.Fatalf("exit %d", code)
		}
		assertGolden(t, "resume.empty.human.golden", normalizeGolden(stdout))
	})

	t.Run("plan_item_start_work_on", func(t *testing.T) {
		stdout, stderr, code := runCLI(t, append(base, "plan", "item", "start", seed.itemID)...)
		if code != 0 {
			t.Fatalf("exit %d stderr=%s", code, stderr)
		}
		assertGolden(t, "plan_item_start.stdout.golden", normalizeGolden(stdout))
		assertGolden(t, "plan_item_start.stderr.golden", normalizeGolden(stderr))
	})

	t.Run("resume_inflight", func(t *testing.T) {
		stdout, _, code := runCLI(t, append(base, "resume")...)
		if code != 0 {
			t.Fatalf("exit %d", code)
		}
		assertGolden(t, "resume.inflight.human.golden", normalizeGolden(stdout))

		jsonOut, _, code := runCLI(t, append(base, "--json", "resume")...)
		if code != 0 {
			t.Fatalf("exit %d", code)
		}
		assertGolden(t, "resume.inflight.json.golden", normalizeGoldenJSON(jsonOut))
	})
}

func TestM13GoldenWrites(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	dbPath := filepath.Join(dir, ".devdb", "development.db")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	base := []string{"--repo", repo, "--db", dbPath}
	_, _, code := runCLI(t, append(base, "init")...)
	if code != 0 {
		t.Fatal("init failed")
	}
	srcFile := filepath.Join(repo, "src.go")
	if err := os.WriteFile(srcFile, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, _, code := runCLI(t, append(base, "inventory", "scan", "--paths", "src.go")...)
	if code != 0 {
		t.Fatalf("scan exit %d stdout=%s", code, stdout)
	}

	type writeCase struct {
		name string
		args []string
	}

	cases := []writeCase{
		{name: "feedback", args: []string{"feedback", "add", "--role", "model", "golden write"}},
		{name: "goal", args: []string{"goal", "add", "Golden goal", "--kind", "goal"}},
		{name: "feature", args: []string{"feature", "add", "Golden feature"}},
		{name: "plan", args: []string{"plan", "create", "Golden plan", "--slug", "golden-write-plan"}},
		{name: "task", args: []string{"task", "add", "Golden task"}},
		{name: "reminder", args: []string{"reminder", "add", "Golden reminder", "--due", "2026-12-31T00:00:00Z"}},
		{name: "arch", args: []string{"arch", "add", "golden-topic", "--body", "Two sentences for the golden fixture test.", "--source", "src.go"}},
		{name: "review", args: []string{"review", "start", "--paths", "."}},
		{name: "verify", args: []string{
			"verify", "record", "go test ./...", "--scope", ".",
			"--git-sha", "0000000000000000000000000000000000000000",
			"--status", "passed", "--exit-code", "0", "--finished",
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name+"_human", func(t *testing.T) {
			stdout, stderr, code := runCLI(t, append(base, tc.args...)...)
			if code != 0 {
				t.Fatalf("exit %d stderr=%s", code, stderr)
			}
			got := normalizeGolden(stdout)
			assertGolden(t, filepath.Join("writes", tc.name+".human.golden"), got)
			if !strings.Contains(got, "<ID>") {
				t.Fatalf("expected normalized id placeholder in %q", got)
			}
		})
		t.Run(tc.name+"_json", func(t *testing.T) {
			jsonArgs := append([]string{"--json"}, tc.args...)
			if tc.name == "plan" {
				// Human test already created the slug; use a distinct slug for JSON golden.
				jsonArgs = []string{"--json", "plan", "create", "Golden plan JSON", "--slug", "golden-write-plan-json"}
			}
			stdout, stderr, code := runCLI(t, append(base, jsonArgs...)...)
			if code != 0 {
				t.Fatalf("exit %d stderr=%s stdout=%s", code, stderr, stdout)
			}
			got := normalizeGoldenJSON(stdout)
			assertGolden(t, filepath.Join("writes", tc.name+".json.golden"), got)
			if !strings.Contains(got, `"id": "<ID>"`) && !strings.Contains(got, `"id":"<ID>"`) {
				t.Fatalf("expected id key in json golden: %q", got)
			}
		})
	}

	// Approval needs an entity; chain off task.
	taskStdout, _, code := runCLI(t, append(base, "task", "add", "Approval task")...)
	if code != 0 {
		t.Fatalf("task add exit %d", code)
	}
	taskID := firstLine(taskStdout)
	approvalCases := []writeCase{
		{name: "approval", args: []string{"approval", "request", taskID}},
	}
	for _, tc := range approvalCases {
		t.Run(tc.name+"_human", func(t *testing.T) {
			stdout, _, code := runCLI(t, append(base, tc.args...)...)
			if code != 0 {
				t.Fatalf("exit %d", code)
			}
			assertGolden(t, filepath.Join("writes", tc.name+".human.golden"), normalizeGolden(stdout))
		})
		t.Run(tc.name+"_json", func(t *testing.T) {
			stdout, _, code := runCLI(t, append(append([]string{}, base...), append([]string{"--json"}, tc.args...)...)...)
			if code != 0 {
				t.Fatalf("exit %d", code)
			}
			assertGolden(t, filepath.Join("writes", tc.name+".json.golden"), normalizeGoldenJSON(stdout))
		})
	}
}
