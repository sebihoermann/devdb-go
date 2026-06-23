package importer

import (
	"path/filepath"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/migrate"
	"github.com/sebihoermann/devdb-go/internal/storage"
)

// TestCopyLegacyDataRollsBackOnTriggerFailure imports a Python-style legacy
// db into a Go-style destination whose `feedback` table has a BEFORE INSERT
// trigger that raises ABORT. The internal copyLegacyData must surface the
// error AND leave the destination with zero rows from the legacy tables —
// the new transactional copyLegacyData rolls back every insert as one unit.
// Regression for review finding ed332eaa.
func TestCopyLegacyDataRollsBackOnTriggerFailure(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "legacy.db")
	if err := writeMinimalPythonDBWithFeedback(src); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(dir, "go.db")
	{
		schemaDB, err := storage.Open(dst)
		if err != nil {
			t.Fatal(err)
		}
		if err := migrate.RunAll(schemaDB); err != nil {
			_ = schemaDB.Close()
			t.Fatal(err)
		}
		// Attach a trigger that forces the feedback copy to fail mid-transaction.
		if _, err := schemaDB.Exec(`
			CREATE TRIGGER feedback_block_insert
			BEFORE INSERT ON feedback
			BEGIN
				SELECT RAISE(ABORT, 'forced feedback failure');
			END`); err != nil {
			_ = schemaDB.Close()
			t.Fatal(err)
		}
		if err := schemaDB.Close(); err != nil {
			t.Fatal(err)
		}
	}

	db, err := storage.Open(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	counts := map[string]int{}
	if err := copyLegacyData(db, src, counts); err == nil {
		t.Fatal("expected copyLegacyData to fail when feedback trigger raises")
	}

	for _, table := range []string{"feedback", "goals", "plans", "milestones", "plan_items", "tasks"} {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatalf("%s rows=%d, want 0 (rollback failed)", table, n)
		}
	}
	// Sanity: the trigger still exists — copyLegacyData's tx must not have
	// dropped it.
	var triggerName string
	if err := db.QueryRow(`SELECT name FROM sqlite_master WHERE name='feedback_block_insert'`).Scan(&triggerName); err != nil {
		t.Fatal(err)
	}
	if triggerName != "feedback_block_insert" {
		t.Fatalf("trigger missing: %q", triggerName)
	}
}