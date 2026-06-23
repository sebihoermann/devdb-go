package archive

import (
	"testing"

	"github.com/sebihoermann/devdb-go/internal/storage"
)

// TestArchiveTableRollsBackOnInsertFailure installs a BEFORE INSERT trigger on
// archive_entries that raises ABORT after features have been selected and the
// archive_entries row is about to be inserted. archiveTable must roll back
// every write (the archive_entries INSERTs and the source DELETE) as one unit.
// Regression for review finding ed332eaa.
func TestArchiveTableRollsBackOnInsertFailure(t *testing.T) {
	db := archiveTestDB(t)
	old := "2020-01-01T00:00:00Z"
	if _, err := db.Exec(`
		INSERT INTO features(id, title, created_at, model_id) VALUES ('f1', 'old', ?, 'test')`,
		old,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO features(id, title, created_at, model_id) VALUES ('f2', 'old', ?, 'test')`,
		old,
	); err != nil {
		t.Fatal(err)
	}

	if _, err := db.Exec(`
		CREATE TRIGGER archive_entries_block_insert
		BEFORE INSERT ON archive_entries
		BEGIN
			SELECT RAISE(ABORT, 'forced archive failure');
		END`); err != nil {
		t.Fatal(err)
	}

	step := archiveStep{table: "features", where: "1=1"}
	if _, err := archiveTable(db, step, storage.NowUTC(), 0); err == nil {
		t.Fatal("expected archiveTable to fail when archive_entries trigger raises")
	}

	// Both source rows must still be present — the DELETE was rolled back.
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM features`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("features rows=%d, want 2 (rollback failed)", n)
	}
	// And the archive_entries INSERTs were rolled back too.
	if err := db.QueryRow(`SELECT COUNT(*) FROM archive_entries`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("archive_entries rows=%d, want 0 (rollback failed)", n)
	}
}

// TestArchiveTableRollsBackOnDeleteFailure installs a trigger on features
// that fails on DELETE. archiveTable must roll back the previously inserted
// archive_entries rows so no orphan archive rows are left behind.
func TestArchiveTableRollsBackOnDeleteFailure(t *testing.T) {
	db := archiveTestDB(t)
	old := "2020-01-01T00:00:00Z"
	if _, err := db.Exec(`
		INSERT INTO features(id, title, created_at, model_id) VALUES ('f1', 'old', ?, 'test')`,
		old,
	); err != nil {
		t.Fatal(err)
	}

	if _, err := db.Exec(`
		CREATE TRIGGER features_block_delete
		BEFORE DELETE ON features
		BEGIN
			SELECT RAISE(ABORT, 'forced delete failure');
		END`); err != nil {
		t.Fatal(err)
	}

	step := archiveStep{table: "features", where: "1=1"}
	if _, err := archiveTable(db, step, storage.NowUTC(), 0); err == nil {
		t.Fatal("expected archiveTable to fail when features delete trigger raises")
	}

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM features`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("features rows=%d, want 1 (rollback failed)", n)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM archive_entries`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("archive_entries rows=%d, want 0 (rollback failed)", n)
	}
}