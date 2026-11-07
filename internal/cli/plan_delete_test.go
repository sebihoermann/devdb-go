package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/storage"
)

// TestPlanDeleteCascade covers feedback af49aa29: before this verb,
// pivoting a project meant hand-ordering a raw-SQL FK cascade
// (acceptance -> files -> status_log -> items -> milestones -> plans,
// skill pitfall 10). The CLI verb must delete the whole subtree in one
// transaction, refuse in_progress plans without --force, and preview
// with --dry-run without writing.
func TestPlanDeleteCascade(t *testing.T) {
	e := newInProcessEnv(t)

	planID := e.run("plan", "create", "Doomed Plan", "--slug", "doomed")
	msID := e.run("plan", "milestone", "add", "Doomed milestone", "--plan", planID)
	itemID := e.run("plan", "item", "add", "Doomed item", "--plan", planID, "--milestone", msID)
	e.run("plan", "acceptance", "add", "--plan-item", itemID, "some criterion")
	e.run("plan", "file", "add", "--plan-item", itemID, "--path", "doomed.go", "--role", "create")
	e.run("plan", "item", "start", itemID) // writes a status_log row
	e.run("plan", "item", "pause", itemID, "--note", "pause for test")

	sibling := e.run("plan", "create", "Sibling Plan", "--slug", "sibling")

	// in_progress refuses without --force (exit 5, actionable message).
	_, stderr, code := runCLI(t, append(e.base, "plan", "delete", "doomed")...)
	if code != 5 {
		t.Fatalf("in_progress plan delete exit=%d want 5; stderr=%s", code, stderr)
	}
	if !strings.Contains(stderr, "in_progress") || !strings.Contains(stderr, "--force") {
		t.Fatalf("refusal should name in_progress + --force: %s", stderr)
	}
	// Refusal wrote nothing.
	if s := planExists(t, e.dbPath, planID); !s {
		t.Fatal("refused delete must leave the plan intact")
	}

	// Dry-run previews counts without writing.
	var dry struct {
		Items      int  `json:"items"`
		Milestones int  `json:"milestones"`
		Acceptance int  `json:"acceptance"`
		Files      int  `json:"files"`
		StatusLog  int  `json:"status_log"`
		DryRun     bool `json:"dry_run"`
	}
	out := e.runOut("--json", "plan", "delete", "doomed", "--dry-run")
	if err := json.Unmarshal([]byte(out), &dry); err != nil {
		t.Fatalf("dry-run json: %v (%s)", err, out)
	}
	if dry.Items != 1 || dry.Milestones != 1 || dry.Acceptance != 1 || dry.Files != 1 || dry.StatusLog < 2 {
		t.Fatalf("dry-run counts wrong: %+v", dry)
	}
	if !dry.DryRun {
		t.Fatal("dry_run flag should be true")
	}
	if s := planExists(t, e.dbPath, planID); !s {
		t.Fatal("dry-run must not write")
	}

	// --force deletes the whole subtree.
	var res struct {
		PlanID string `json:"plan_id"`
		Items  int    `json:"items"`
	}
	out = e.runOut("--json", "plan", "delete", "doomed", "--force")
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("delete json: %v (%s)", err, out)
	}
	if res.PlanID != planID {
		t.Fatalf("deleted plan %q want %q", res.PlanID, planID)
	}
	if planExists(t, e.dbPath, planID) {
		t.Fatal("plan row still present after delete")
	}
	if n := orphanCount(t, e.dbPath, "milestones", "plan_id", planID); n != 0 {
		t.Fatalf("%d orphan milestones", n)
	}
	if n := orphanCount(t, e.dbPath, "plan_items", "plan_id", planID); n != 0 {
		t.Fatalf("%d orphan plan_items", n)
	}
	if n := countRows(t, e.dbPath, "plan_item_acceptance"); n != 0 {
		t.Fatalf("%d orphan acceptance rows", n)
	}
	if n := countRows(t, e.dbPath, "plan_item_files"); n != 0 {
		t.Fatalf("%d orphan plan_item_files rows", n)
	}
	if n := orphanCount(t, e.dbPath, "status_log", "plan_item_id", itemID); n != 0 {
		t.Fatalf("%d orphan status_log rows", n)
	}
	// Sibling untouched.
	if !planExists(t, e.dbPath, sibling) {
		t.Fatal("sibling plan was collateral damage")
	}

	// Deleting an unknown plan fails loudly (exit 3).
	if _, _, code := runCLI(t, append(e.base, "plan", "delete", "no-such-plan")...); code != 3 {
		t.Fatalf("unknown plan delete exit=%d want 3", code)
	}

	// A clean (no in_progress) plan deletes without --force.
	clean := e.run("plan", "create", "Clean Plan", "--slug", "clean")
	e.run("plan", "delete", "clean")
	if planExists(t, e.dbPath, clean) {
		t.Fatal("clean plan should delete without --force")
	}
}

// TestGoalDelete covers the goal delete verb (feedback af49aa29): goals
// have no children, so delete is a single row removal with prefix
// resolution; unknown prefix fails loudly.
func TestGoalDelete(t *testing.T) {
	e := newInProcessEnv(t)
	gid := e.run("goal", "add", "goal", "Finish the thing")
	keeper := e.run("goal", "add", "do", "Keep this one")

	e.run("goal", "delete", gid[:8]) // prefix resolution like every other verb
	if n := goalExists(t, e.dbPath, gid); n {
		t.Fatal("goal still present after delete")
	}
	if !goalExists(t, e.dbPath, keeper) {
		t.Fatal("unrelated goal was deleted")
	}
	if _, _, code := runCLI(t, append(e.base, "goal", "delete", "deadbeef99")...); code == 0 {
		t.Fatal("unknown goal prefix should exit non-zero")
	}
}

func planExists(t *testing.T, dbPath, planID string) bool {
	t.Helper()
	return countRowsWhere(t, dbPath, "plans", "id", planID) > 0
}

func goalExists(t *testing.T, dbPath, goalID string) bool {
	t.Helper()
	return countRowsWhere(t, dbPath, "goals", "id", goalID) > 0
}

func orphanCount(t *testing.T, dbPath, table, col, val string) int {
	t.Helper()
	return countRowsWhere(t, dbPath, table, col, val)
}

func countRowsWhere(t *testing.T, dbPath, table, col, val string) int {
	t.Helper()
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE `+col+`=?`, val).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func countRows(t *testing.T, dbPath, table string) int {
	t.Helper()
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
