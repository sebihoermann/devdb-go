package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestM9PlanningCommands(t *testing.T) {
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
		stdout, stderr, code := runDevdb(t, bin, all...)
		if code != 0 {
			t.Fatalf("%v exit %d stderr=%s stdout=%s", args, code, stderr, stdout)
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

	planID := run("plan", "scaffold", "M9 Feature", "--slug", "m9-test", "--mode", "design", "--milestones", "2")
	if planID == "" {
		t.Fatal("scaffold returned empty id")
	}

	artifact := filepath.Join(repo, "docs", "m9-test-implementation-plan.html")
	if _, err := os.Stat(artifact); err != nil {
		t.Fatalf("scaffold artifact missing: %v", err)
	}

	promoteID := run("plan", "promote", "--plan", "m9-test")
	if promoteID != planID {
		t.Fatalf("promote id %s != plan %s", promoteID, planID)
	}

	driftOut := runOut("plan", "reconcile")
	if !strings.Contains(driftOut, "drift detected") && !strings.Contains(driftOut, "no plan-tree drift") {
		t.Fatalf("unexpected reconcile output: %q", driftOut)
	}

	specPath := filepath.Join(repo, "SPEC.md")
	spec := `### M1 Alpha

- [ ] first check
- [ ] second check
`
	if err := os.WriteFile(specPath, []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}
	backfillOut := runOut("plan", "acceptance", "backfill", "--milestone", "M1", "--spec", specPath)
	if !strings.Contains(backfillOut, "created 2 acceptance") {
		t.Fatalf("backfill output: %q", backfillOut)
	}

	jsonBackfill := runOut("--json", "plan", "acceptance", "backfill", "--milestone", "M1", "--spec", specPath)
	if !strings.Contains(jsonBackfill, `"created":0`) && !strings.Contains(jsonBackfill, `"created": 0`) {
		t.Fatalf("json backfill idempotent: %q", jsonBackfill)
	}

	scaffoldJSON := runOut("--json", "plan", "scaffold", "JSON plan", "--slug", "json-plan", "--milestones", "1")
	if !strings.Contains(scaffoldJSON, `"plan_id"`) && !strings.Contains(scaffoldJSON, planID[:8]) {
		// WriteResult with --json prints {"id":...} plus metadata
		if !strings.Contains(scaffoldJSON, `"id"`) {
			t.Fatalf("scaffold json: %q", scaffoldJSON)
		}
	}
}

func TestM9ScaffoldDuplicateSlug(t *testing.T) {
	bin := buildDevdb(t)
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	dbPath := filepath.Join(dir, ".devdb", "development.db")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	_, _, code := runDevdb(t, bin, "--repo", repo, "--db", dbPath, "init")
	if code != 0 {
		t.Fatal("init failed")
	}
	_, _, code = runDevdb(t, bin, "--repo", repo, "--db", dbPath, "plan", "scaffold", "One", "--slug", "dup")
	if code != 0 {
		t.Fatal("first scaffold failed")
	}
	_, stderr, code := runDevdb(t, bin, "--repo", repo, "--db", dbPath, "plan", "scaffold", "Two", "--slug", "dup")
	if code == 0 {
		t.Fatal("expected duplicate slug failure")
	}
	if !strings.Contains(stderr, "slug already exists") {
		t.Fatalf("stderr=%q", stderr)
	}
}

func TestM9LegacyCommandSuggestions(t *testing.T) {
	bin := buildDevdb(t)
	_, stderr, code := runDevdb(t, bin, "scaffold-plan", "Title")
	if code == 0 {
		t.Fatal("expected unknown command failure")
	}
	if !strings.Contains(stderr, "devdb plan scaffold") {
		t.Fatalf("missing suggestion: %q", stderr)
	}
}

func TestPlanItemShowIncludesMemoryRef(t *testing.T) {
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
		stdout, stderr, code := runDevdb(t, bin, all...)
		if code != 0 {
			t.Fatalf("%v exit %d stderr=%s stdout=%s", args, code, stderr, stdout)
		}
		return firstLine(stdout)
	}

	runOut := func(args ...string) string {
		t.Helper()
		all := append([]string{"--repo", repo, "--db", dbPath}, args...)
		stdout, stderr, code := runDevdb(t, bin, all...)
		if code != 0 {
			t.Fatalf("%v exit %d stderr=%s stdout=%s", args, code, stderr, stdout)
		}
		return stdout
	}

	run("init")
	planID := run("plan", "create", "Memory plan", "--slug", "memory-plan")
	msID := run("plan", "milestone", "add", "--plan", planID, "Memory milestone")
	itemID := run("plan", "item", "add", "--plan", planID, "--milestone", msID, "--memory-ref", "MEMORY.md#anchor", "Use memory ref")

	showOut := runOut("plan", "item", "show", itemID)
	if !strings.Contains(showOut, "memory_ref: MEMORY.md#anchor") {
		t.Fatalf("show output: %q", showOut)
	}

	jsonOut := runOut("--json", "plan", "item", "show", itemID)
	if !strings.Contains(jsonOut, `"memory_ref": "MEMORY.md#anchor"`) {
		t.Fatalf("json output: %q", jsonOut)
	}
}
