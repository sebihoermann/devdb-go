package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// groupCommands is every parent command that dispatches to subcommands. The
// acceptance matrix below walks this list, so a new group added without
// failClosed() fails the suite rather than silently reintroducing the
// help-fallback-exits-0 bug.
var groupCommands = []string{
	"plan", "feedback", "goal", "feature", "task", "reminder", "approval",
	"plan milestone", "plan item", "plan acceptance", "plan file",
	"archive", "analytics", "ledger", "hub", "inventory", "arch",
	"review", "verify",
}

// TestUnknownSubcommandFailsClosed is the matrix required by acceptance
// criterion 4: for every group command, an unrecognised subcommand must exit
// non-zero. Before this, cobra printed the parent help and exited 0, so
// `devdb plan acceptance list` (a verb that did not exist) told a script it had
// succeeded and produced no rows.
func TestUnknownSubcommandFailsClosed(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "development.db")
	runCLIOut(t, "--db", dbPath, "init")

	for _, group := range groupCommands {
		t.Run(group, func(t *testing.T) {
			args := append(strings.Fields(group), "definitely-not-a-verb")
			full := append([]string{"--db", dbPath}, args...)
			stdout, stderr, code := runCLI(t, full...)
			if code == 0 {
				t.Fatalf("%v exited 0; an unknown subcommand must fail (stdout=%q stderr=%q)",
					args, stdout, stderr)
			}
			msg := stdout + stderr
			if !strings.Contains(msg, "definitely-not-a-verb") {
				t.Fatalf("%v message = %q, want it to name the unknown subcommand", args, msg)
			}
			// The parent help dump must not be what a typo produces; the error
			// should point at help instead of printing the whole command list.
			if strings.Contains(msg, "Available Commands:") {
				t.Fatalf("%v printed the full help tree instead of an error", args)
			}
		})
	}
}

// TestBareGroupStillPrintsHelp keeps discovery intact: the fail-closed guard
// must not make `devdb plan` itself an error. Only an unrecognised word fails.
func TestBareGroupStillPrintsHelp(t *testing.T) {
	for _, group := range []string{"plan", "feedback", "goal", "hub", "analytics"} {
		t.Run(group, func(t *testing.T) {
			stdout, stderr, code := runCLI(t, strings.Fields(group)...)
			if code != 0 {
				t.Fatalf("bare %q exited %d, want 0 (stdout=%q stderr=%q)", group, code, stdout, stderr)
			}
			if !strings.Contains(stdout, "Available Commands:") && !strings.Contains(stdout, "Usage:") {
				t.Fatalf("bare %q printed no help (stdout=%q)", group, stdout)
			}
		})
	}
}

// TestPlanAcceptanceListByItem covers acceptance 1: the new read verb returns
// the criteria rows for one item.
func TestPlanAcceptanceListByItem(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "development.db")
	runCLIOut(t, "--db", dbPath, "init")

	planID := strings.TrimSpace(runCLIOut(t, "--db", dbPath, "plan", "create", "Acceptance list plan", "--slug", "acc-list"))
	itemID := strings.TrimSpace(runCLIOut(t, "--db", dbPath, "plan", "item", "add", "Item with criteria", "--plan", planID))
	runCLIOut(t, "--db", dbPath, "plan", "acceptance", "add", "--plan-item", itemID, "first criterion")
	acc2 := strings.TrimSpace(runCLIOut(t, "--db", dbPath, "plan", "acceptance", "add", "--plan-item", itemID, "second criterion"))
	runCLIOut(t, "--db", dbPath, "plan", "acceptance", "meet", acc2, "--evidence", "proved it")

	out := runCLIOut(t, "--db", dbPath, "plan", "acceptance", "list", "--plan-item", itemID, "--json")
	var rows []struct {
		ID        string `json:"id"`
		Ordinal   int    `json:"ordinal"`
		Criterion string `json:"criterion"`
		Status    string `json:"status"`
		Evidence  string `json:"evidence"`
	}
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("acceptance list is not JSON: %v\n%s", err, out)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d criteria, want 2", len(rows))
	}
	if rows[0].Criterion != "first criterion" || rows[0].Ordinal != 1 || rows[0].Status != "open" {
		t.Fatalf("row 0 = %+v, want the first open criterion", rows[0])
	}
	if rows[1].Criterion != "second criterion" || rows[1].Status != "met" || rows[1].Evidence != "proved it" {
		t.Fatalf("row 1 = %+v, want the met criterion with evidence", rows[1])
	}

	// Human mode must show both without --json.
	human := runCLIOut(t, "--db", dbPath, "plan", "acceptance", "list", "--plan-item", itemID)
	if !strings.Contains(human, "first criterion") || !strings.Contains(human, "second criterion") {
		t.Fatalf("human acceptance list missing criteria: %q", human)
	}
	if !strings.Contains(human, "[x]") || !strings.Contains(human, "[ ]") {
		t.Fatalf("human acceptance list should mark met vs open: %q", human)
	}
}

// TestPlanAcceptanceListByPlan covers the whole-plan read plus --open.
func TestPlanAcceptanceListByPlan(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "development.db")
	runCLIOut(t, "--db", dbPath, "init")

	planID := strings.TrimSpace(runCLIOut(t, "--db", dbPath, "plan", "create", "Whole plan", "--slug", "whole-plan"))
	itemA := strings.TrimSpace(runCLIOut(t, "--db", dbPath, "plan", "item", "add", "Item A", "--plan", planID))
	itemB := strings.TrimSpace(runCLIOut(t, "--db", dbPath, "plan", "item", "add", "Item B", "--plan", planID))
	runCLIOut(t, "--db", dbPath, "plan", "acceptance", "add", "--plan-item", itemA, "A criterion")
	metID := strings.TrimSpace(runCLIOut(t, "--db", dbPath, "plan", "acceptance", "add", "--plan-item", itemB, "B criterion"))
	runCLIOut(t, "--db", dbPath, "plan", "acceptance", "meet", metID, "--evidence", "done")

	all := runCLIOut(t, "--db", dbPath, "plan", "acceptance", "list", "--plan", "whole-plan", "--json")
	var rows []map[string]any
	if err := json.Unmarshal([]byte(all), &rows); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, all)
	}
	if len(rows) != 2 {
		t.Fatalf("--plan returned %d criteria, want 2", len(rows))
	}

	open := runCLIOut(t, "--db", dbPath, "plan", "acceptance", "list", "--plan", "whole-plan", "--open", "--json")
	var openRows []map[string]any
	if err := json.Unmarshal([]byte(open), &openRows); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, open)
	}
	if len(openRows) != 1 {
		t.Fatalf("--open returned %d criteria, want 1", len(openRows))
	}
	if openRows[0]["criterion"] != "A criterion" {
		t.Fatalf("--open returned the wrong row: %+v", openRows[0])
	}
}

// TestPlanAcceptanceListUnknownPlanFails covers acceptance 2: a plan that does
// not exist must be a non-zero "plan not found", not an empty success.
func TestPlanAcceptanceListUnknownPlanFails(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "development.db")
	runCLIOut(t, "--db", dbPath, "init")

	stdout, stderr, code := runCLI(t, "--db", dbPath, "plan", "acceptance", "list", "--plan", "no-such-plan")
	if code == 0 {
		t.Fatalf("unknown --plan exited 0 (stdout=%q stderr=%q)", stdout, stderr)
	}
	msg := strings.ToLower(stdout + stderr)
	if !strings.Contains(msg, "not found") {
		t.Fatalf("message = %q, want it to say the plan was not found", msg)
	}
}

// TestPlanAcceptanceListRejectsBadUsage covers acceptance 3's sibling cases: a
// missing selector, conflicting selectors, and a stray positional arg are all
// usage errors rather than an empty success.
func TestPlanAcceptanceListRejectsBadUsage(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "development.db")
	runCLIOut(t, "--db", dbPath, "init")

	planID := strings.TrimSpace(runCLIOut(t, "--db", dbPath, "plan", "create", "Usage plan", "--slug", "usage-plan"))

	cases := []struct {
		name string
		args []string
	}{
		{"no selector", []string{"plan", "acceptance", "list"}},
		{"both selectors", []string{"plan", "acceptance", "list", "--plan", planID, "--plan-item", "abc"}},
		{"stray positional", []string{"plan", "acceptance", "list", planID}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			full := append([]string{"--db", dbPath}, tc.args...)
			stdout, stderr, code := runCLI(t, full...)
			if code != 2 {
				t.Fatalf("%v exit code = %d, want 2 (stdout=%q stderr=%q)", tc.args, code, stdout, stderr)
			}
		})
	}
}

// TestPlanAcceptanceListEmptyPlanPhrase keeps the 2.3 phrasing contract.
func TestPlanAcceptanceListEmptyPlanPhrase(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "development.db")
	runCLIOut(t, "--db", dbPath, "init")
	runCLIOut(t, "--db", dbPath, "plan", "create", "Empty plan", "--slug", "empty-plan")

	human := strings.TrimSpace(runCLIOut(t, "--db", dbPath, "plan", "acceptance", "list", "--plan", "empty-plan"))
	if human != "no acceptance criteria" {
		t.Fatalf("empty plan acceptance (human) = %q", human)
	}
	jsonOut := strings.TrimSpace(runCLIOut(t, "--db", dbPath, "plan", "acceptance", "list", "--plan", "empty-plan", "--json"))
	if jsonOut != "[]" {
		t.Fatalf("empty plan acceptance --json = %q, want []", jsonOut)
	}
}
