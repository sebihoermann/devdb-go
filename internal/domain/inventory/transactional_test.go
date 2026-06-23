package inventory_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/domain/inventory"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

// TestScanRollsBackOnScanRunsFailure installs a BEFORE INSERT trigger on
// scan_runs that raises ABORT. inventory.Scan must roll back its repo_files
// inserts/updates and file_change_events inserts so the failed scan leaves
// no partial state. Regression for review finding ded0572a.
func TestScanRollsBackOnScanRunsFailure(t *testing.T) {
	db, _ := testutil.TempDB(t)
	repo := t.TempDir()
	for _, rel := range []string{"pkg/a.go", "pkg/b.go"} {
		p := filepath.Join(repo, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("package a\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := db.Exec(`
		CREATE TRIGGER scan_runs_block_insert
		BEFORE INSERT ON scan_runs
		BEGIN
			SELECT RAISE(ABORT, 'forced scan_runs failure');
		END`); err != nil {
		t.Fatal(err)
	}

	if _, err := inventory.Scan(db, repo, nil, false, "test"); err == nil {
		t.Fatal("expected Scan to fail when scan_runs trigger raises")
	}

	// repo_files must be empty — the partial repo_files inserts were rolled back.
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM repo_files`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("repo_files rows=%d, want 0 (rollback failed)", n)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM scan_runs`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("scan_runs rows=%d, want 0 (rollback failed)", n)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM file_change_events`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("file_change_events rows=%d, want 0 (rollback failed)", n)
	}
}

// TestScanRollsBackOnChangeEventsFailure installs a trigger on
// file_change_events that fails after repo_files and scan_runs are written.
// All three tables must end up empty because the entire scan is one tx.
func TestScanRollsBackOnChangeEventsFailure(t *testing.T) {
	db, _ := testutil.TempDB(t)
	repo := t.TempDir()
	for _, rel := range []string{"pkg/a.go"} {
		p := filepath.Join(repo, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("package a\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := db.Exec(`
		CREATE TRIGGER file_change_events_block_insert
		BEFORE INSERT ON file_change_events
		BEGIN
			SELECT RAISE(ABORT, 'forced events failure');
		END`); err != nil {
		t.Fatal(err)
	}

	if _, err := inventory.Scan(db, repo, nil, false, "test"); err == nil {
		t.Fatal("expected Scan to fail when file_change_events trigger raises")
	}

	for _, table := range []string{"repo_files", "scan_runs", "file_change_events"} {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM `+table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatalf("%s rows=%d, want 0 (rollback failed)", table, n)
		}
	}
}

// TestScanHappyPathStillRecords confirms the transactional refactor did not
// regress the common case — repo_files, scan_runs, and file_change_events
// all populate on success.
func TestScanHappyPathStillRecords(t *testing.T) {
	db, _ := testutil.TempDB(t)
	repo := t.TempDir()
	for _, rel := range []string{"pkg/a.go", "pkg/b.go"} {
		p := filepath.Join(repo, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("package a\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	res, err := inventory.Scan(db, repo, nil, false, "test")
	if err != nil {
		t.Fatal(err)
	}
	if res.FilesAdded != 2 {
		t.Fatalf("files_added=%d, want 2", res.FilesAdded)
	}
	for _, table := range []string{"repo_files", "scan_runs", "file_change_events"} {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM `+table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			t.Fatalf("%s rows=0, want >0", table)
		}
	}
}