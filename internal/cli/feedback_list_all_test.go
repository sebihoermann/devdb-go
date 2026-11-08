package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// TestFeedbackListStatusAllReturnsEveryStatus is a regression for the
// dogfood finding 9fa4900a983dafb402ae5ea45b7e3122 (severity med):
// `devdb feedback list --status all` documented "all" as "no status
// filter", but the value was passed through to the domain layer literally,
// so the query filtered for status='all' and matched nothing — the list came
// back empty while the ledger held rows. The sibling verbs (goal, task,
// reminder, plan item, review) already honoured the sentinel; feedback was
// the odd one out.
func TestFeedbackListStatusAllReturnsEveryStatus(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "development.db")
	runCLIOut(t, "--db", dbPath, "init")

	// One open row and one closed row.
	openID := strings.TrimSpace(runCLIOut(t, "--db", dbPath, "feedback", "add", "open note", "--role", "model"))
	closedID := strings.TrimSpace(runCLIOut(t, "--db", dbPath, "feedback", "add", "closed note", "--role", "model"))
	runCLIOut(t, "--db", dbPath, "feedback", "close", closedID, "--proposed-fix", "fixed")

	// --status all must return every row, whatever its status.
	all := decodeFeedbackRows(t, runCLIOut(t, "--db", dbPath, "feedback", "list", "--status", "all"))
	if len(all) != 2 {
		t.Fatalf("feedback list --status all: got %d rows, want 2", len(all))
	}

	// The individual filters must keep working.
	onlyOpen := decodeFeedbackRows(t, runCLIOut(t, "--db", dbPath, "feedback", "list", "--status", "open"))
	if len(onlyOpen) != 1 || onlyOpen[0]["id"] != openID {
		t.Fatalf("feedback list --status open: got %v", onlyOpen)
	}
	onlyClosed := decodeFeedbackRows(t, runCLIOut(t, "--db", dbPath, "feedback", "list", "--status", "closed"))
	if len(onlyClosed) != 1 || onlyClosed[0]["id"] != closedID {
		t.Fatalf("feedback list --status closed: got %v", onlyClosed)
	}
}

// TestEmptyListPrintsEmptyArrayInHumanMode is a regression for the same
// finding: with the default (non---json) output, an empty list printed
// "null" while the --json path printed "[]". An empty result and a filter
// that matched nothing must not look the same to a caller.
//
// The human branch now prints a phrase ("no feedback") rather than "[]":
// "[]" is JSON leaking into the human view, and the phrase is what the
// improvements plan asks a human-mode list to say. The contract this test
// actually protects is unchanged — human output must never be the bare
// null/[] that made an empty result and an unmatched filter
// indistinguishable — so the assertion is on the phrase, and the JSON
// branch is asserted separately to still be exactly [].
func TestEmptyListPrintsEmptyArrayInHumanMode(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "development.db")
	runCLIOut(t, "--db", dbPath, "init")

	out := strings.TrimSpace(runCLIOut(t, "--db", dbPath, "feedback", "list", "--status", "open"))
	if out == "" || out == "null" || out == "[]" {
		t.Fatalf("empty feedback list (human) = %q, want a human phrase", out)
	}
	if !strings.Contains(out, "no feedback") {
		t.Fatalf("empty feedback list (human) = %q, want it to say %q", out, "no feedback")
	}

	// The machine branch keeps the stable empty-array shape.
	jsonOut := strings.TrimSpace(runCLIOut(t, "--db", dbPath, "feedback", "list", "--status", "open", "--json"))
	if jsonOut != "[]" {
		t.Fatalf("empty feedback list --json = %q, want []", jsonOut)
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(jsonOut), &rows); err != nil {
		t.Fatalf("empty feedback list --json is not valid JSON: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("empty feedback list --json decoded to %d rows", len(rows))
	}
}

// decodeFeedbackRows parses a JSON array printed by a feedback list verb.
// Named distinctly from other helpers in this package so this test file
// stays self-contained and the branch remains independently mergeable.
func decodeFeedbackRows(t *testing.T, out string) []map[string]any {
	t.Helper()
	out = strings.TrimSpace(out)
	if out == "" {
		return nil
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("decode %q: %v", out, err)
	}
	return rows
}
