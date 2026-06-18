package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDogfoodSession(t *testing.T) {
	bin := buildDevdb(t)
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	dbPath := filepath.Join(dir, ".devdb", "development.db")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}

	run := func(args ...string) string {
		t.Helper()
		all := append([]string{"--repo", repo, "--db", dbPath}, args...)
		stdout, _, code := runDevdb(t, bin, all...)
		if code != 0 {
			t.Fatalf("%v exit %d stdout=%s", args, code, stdout)
		}
		return firstLine(stdout)
	}

	runOut := func(args ...string) string {
		t.Helper()
		all := append([]string{"--repo", repo, "--db", dbPath}, args...)
		stdout, _, code := runDevdb(t, bin, all...)
		if code != 0 {
			t.Fatalf("%v exit %d", args, code)
		}
		return stdout
	}

	run("init")

	planID := run("plan", "create", "Dogfood plan", "--slug", "dogfood")
	msID := run("plan", "milestone", "add", "M1 Core", "--plan", planID)
	itemID := run("plan", "item", "add", "Ship workflow", "--plan", planID, "--milestone", msID)
	accID := run("plan", "acceptance", "add", "workflow works end-to-end", "--plan-item", itemID)
	_ = run("plan", "file", "add", "--plan-item", itemID, "--path", "golang/", "--role", "modify")
	_ = run("feedback", "add", "--role", "model", "dogfood session note")
	_ = run("goal", "add", "Complete M3", "--kind", "goal")
	_ = run("task", "add", "Verify dogfood test")
	_ = run("reminder", "add", "Re-run dogfood", "--due", "2026-12-31T00:00:00Z")

	started := run("plan", "item", "start", itemID)
	if started != itemID {
		t.Fatalf("start id mismatch %s vs %s", started, itemID)
	}

	if out := runOut("resume"); !strings.Contains(out, "Ship workflow") {
		t.Fatalf("resume missing in-flight work: %q", out)
	}

	run("plan", "item", "pause", itemID, "--note", "mid-session checkpoint")
	run("plan", "acceptance", "meet", accID, "--evidence", "dogfood test")
	closed := run("plan", "item", "close", itemID, "--evidence", "dogfood session")

	if closed != itemID {
		t.Fatalf("close id mismatch")
	}

	legacyID := run("plan", "item", "add", "Legacy item", "--legacy", "--phase", "M9", "--step", "1")
	listOut := runOut("plan", "item", "list", "--legacy")
	if !strings.Contains(listOut, legacyID[:8]) && !strings.Contains(listOut, "Legacy") {
		// JSON list contains title
		if !strings.Contains(listOut, "Legacy item") {
			t.Fatalf("legacy list missing item: %s", listOut)
		}
	}

	statusOut := runOut("status")
	if strings.Count(statusOut, "\n") > 8 {
		t.Fatalf("status should be compact (<8 lines), got %d lines: %q", strings.Count(statusOut, "\n")+1, statusOut)
	}

	verboseOut := runOut("status", "--verbose")
	if !strings.Contains(verboseOut, "schema:") {
		t.Fatalf("verbose status missing schema detail: %q", verboseOut)
	}
}

func TestLegacyPlanItemReadable(t *testing.T) {
	bin := buildDevdb(t)
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "development.db")
	runDevdb(t, bin, "--db", dbPath, "init")
	stdout, _, code := runDevdb(t, bin, "--db", dbPath, "plan", "item", "add", "Old style", "--legacy", "--phase", "M1", "--step", "2")
	if code != 0 {
		t.Fatal("legacy add failed")
	}
	itemID := strings.TrimSpace(stdout)
	show, _, code := runDevdb(t, bin, "--db", dbPath, "plan", "item", "show", itemID)
	if code != 0 || !strings.Contains(show, "legacy: M1.2") {
		t.Fatalf("legacy show: %q", show)
	}
}

// Ensure buildDevdb from commands_test is reused.
var _ = exec.Command
