package cli

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// execLedger runs a statement directly against the ledger, for fixture steps
// the CLI has no verb for (backdating created_at so the sweep will take a row).
func execLedger(t *testing.T, dbPath, stmt string, args ...any) {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(stmt, args...); err != nil {
		t.Fatalf("exec %q: %v", stmt, err)
	}
}

// TestPlanItemShowArchivedPrefix is acceptance 2 at the CLI boundary: an id
// that only exists in archive_entries prints [archived] plus a restore hint
// instead of "no match for id prefix".
func TestPlanItemShowArchivedPrefix(t *testing.T) {
	bin := buildDevdb(t)
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	dbPath := filepath.Join(dir, ".devdb", "development.db")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}

	out := func(args ...string) (string, string, int) {
		t.Helper()
		all := append([]string{"--repo", repo, "--db", dbPath}, args...)
		return runDevdb(t, bin, all...)
	}
	mustOut := func(args ...string) string {
		t.Helper()
		stdout, stderr, code := out(args...)
		if code != 0 {
			t.Fatalf("%v exit %d stderr=%s stdout=%s", args, code, stderr, stdout)
		}
		return stdout
	}

	mustOut("init")
	mustOut("plan", "create", "archive show plan", "--slug", "show-plan")
	msID := strings.TrimSpace(mustOut("plan", "milestone", "add", "M1", "--plan", "show-plan"))
	itemID := strings.TrimSpace(mustOut("plan", "item", "add", "swept item", "--plan", "show-plan", "--milestone", msID))
	mustOut("plan", "item", "status", itemID, "done", "--note", "finished")

	// Capture the live output so we can prove it does not change.
	liveBefore := mustOut("plan", "item", "show", itemID)

	execLedger(t, dbPath, `UPDATE plan_items SET created_at='2020-01-01T00:00:00Z'`)
	mustOut("archive", "run", "--table", "plan_items", "--yes")

	// Acceptance 3: identical when the row is still live. Re-create a live
	// item so this check runs against the same binary and ledger.
	secondMS := strings.TrimSpace(mustOut("plan", "milestone", "add", "M2", "--plan", "show-plan"))
	liveID := strings.TrimSpace(mustOut("plan", "item", "add", "live item", "--plan", "show-plan", "--milestone", secondMS))
	liveAfter := mustOut("plan", "item", "show", liveID)
	if strings.Contains(liveAfter, "[archived]") || strings.Contains(liveAfter, "source: archive") {
		t.Fatalf("live item rendered as archived:\n%s", liveAfter)
	}
	if !strings.HasPrefix(liveAfter, "# live item\nstatus: planned · id: "+liveID) {
		t.Fatalf("live item output shape changed:\n%s", liveAfter)
	}
	if !strings.HasPrefix(liveBefore, "# swept item\nstatus: done · id: "+itemID) {
		t.Fatalf("live output before archiving had unexpected shape:\n%s", liveBefore)
	}

	// Acceptance 1: archive show by archive-entry id.
	var entryID string
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT id FROM archive_entries WHERE source_id=?`, itemID).Scan(&entryID); err != nil {
		db.Close()
		t.Fatalf("item was not archived: %v", err)
	}
	db.Close()

	shown := mustOut("archive", "show", entryID)
	for _, want := range []string{"source: archive", itemID, "swept item", "archive restore --source-id"} {
		if !strings.Contains(shown, want) {
			t.Fatalf("archive show missing %q:\n%s", want, shown)
		}
	}

	// Acceptance 2: the archived prefix through plan item show.
	archived := mustOut("plan", "item", "show", itemID[:8])
	for _, want := range []string{"[archived]", "source: archive", "archive restore --source-id " + itemID} {
		if !strings.Contains(archived, want) {
			t.Fatalf("archived show missing %q:\n%s", want, archived)
		}
	}

	// A truly unknown id still fails properly.
	if _, stderr, code := out("plan", "item", "show", "zzzzzzzz"); code == 0 {
		t.Fatalf("unknown id exited 0 (stderr=%s)", stderr)
	}
}
