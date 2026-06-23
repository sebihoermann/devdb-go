package archive

import (
	"strings"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/storage"
)

func TestArchiveTableRejectsMissingTable(t *testing.T) {
	db := archiveTestDB(t)
	_, err := archiveTable(db, archiveStep{table: "missing_table", where: "1=1", reason: "x"}, storage.NowUTC(), 3)
	if err == nil {
		t.Fatal("expected archiveTable error for missing table")
	}
}

func TestArchiveTableRejectsBadQuery(t *testing.T) {
	db := archiveTestDB(t)
	_, err := archiveTable(db, archiveStep{table: "features", where: "not_a_column=?", params: []any{"x"}, reason: "x"}, storage.NowUTC(), 3)
	if err == nil {
		t.Fatal("expected archiveTable query error")
	}
}

func TestCountRowsRejectsBadQuery(t *testing.T) {
	db := archiveTestDB(t)
	_, err := countRows(db, "features", "not_a_column=1", nil, "", 3)
	if err == nil {
		t.Fatal("expected countRows error")
	}
}

func TestArchiveOrphansMissingTable(t *testing.T) {
	db := archiveTestDB(t)
	n, err := archiveOrphans(db, "missing_child_table", storage.NowUTC())
	if err != nil || n != 0 {
		t.Fatalf("missing table orphans=%d err=%v", n, err)
	}
}

func TestListRejectsBadQuery(t *testing.T) {
	db := archiveTestDB(t)
	if _, err := db.Exec(`DROP TABLE archive_entries`); err != nil {
		t.Fatal(err)
	}
	_, err := List(db, ListFilter{Limit: 1})
	if err == nil {
		t.Fatal("expected list error after dropping archive_entries")
	}
}

func TestRestoreRejectsBadQuery(t *testing.T) {
	db := archiveTestDB(t)
	if _, err := db.Exec(`DROP TABLE archive_entries`); err != nil {
		t.Fatal(err)
	}
	_, err := Restore(db, RestoreOptions{Table: "features"})
	if err == nil {
		t.Fatal("expected restore query error")
	}
}

func TestRestoreRejectsInvalidArchivedSourceTable(t *testing.T) {
	db := archiveTestDB(t)
	archID, _ := storage.NewID()
	if _, err := db.Exec(`
		INSERT INTO archive_entries(id, source_table, source_id, payload_json, archived_at)
		VALUES (?, 'features; DROP TABLE feedback; --', 'x', '{"id":"x"}', ?)`, archID, storage.NowUTC()); err != nil {
		t.Fatal(err)
	}
	_, err := Restore(db, RestoreOptions{ID: archID})
	if err == nil || !strings.Contains(err.Error(), "invalid archived source table") {
		t.Fatalf("err=%v", err)
	}
}

func TestRestoreRejectsInvalidArchivedColumn(t *testing.T) {
	db := archiveTestDB(t)
	archID, _ := storage.NewID()
	if _, err := db.Exec(`
		INSERT INTO archive_entries(id, source_table, source_id, payload_json, archived_at)
		VALUES (?, 'features', 'x', '{"id":"x","title":"feat","bad) VALUES (1); --":"boom"}', ?)`, archID, storage.NowUTC()); err != nil {
		t.Fatal(err)
	}
	_, err := Restore(db, RestoreOptions{ID: archID})
	if err == nil || !strings.Contains(err.Error(), "invalid archived column") {
		t.Fatalf("err=%v", err)
	}
}

func TestGCRejectsBadFeedbackQuery(t *testing.T) {
	db := archiveTestDB(t)
	if _, err := db.Exec(`DROP TABLE feedback`); err != nil {
		t.Fatal(err)
	}
	_, err := GC(db, GCOptions{DryRun: true})
	if err == nil {
		t.Fatal("expected gc error when feedback table missing")
	}
}
