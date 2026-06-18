package archive

import (
	"testing"
	"time"

	"github.com/sebihoermann/devdb-go/internal/storage"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestArchiveTableDirectAndReadRows(t *testing.T) {
	db, _ := testutil.TempDB(t)
	old := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO features(id, title, created_at, model_id) VALUES ('f-arch','feat',?,'test')`, old); err != nil {
		t.Fatal(err)
	}
	now := storage.NowUTC()
	n, err := archiveTable(db, archiveStep{
		table: "features", where: "created_at < ?", params: []any{now}, reason: "retention",
	}, now, 3)
	if err != nil || n != 1 {
		t.Fatalf("archive=%d err=%v", n, err)
	}
	rows, err := readTableRows(db, `SELECT * FROM archive_entries WHERE source_table='features'`)
	if err != nil || len(rows) != 1 {
		t.Fatalf("archived rows=%d err=%v", len(rows), err)
	}
	if _, err := readTableRows(db, "SELECT FROM bad_syntax"); err == nil {
		t.Fatal("expected query error")
	}
}

func TestCoverageFinalArchiveRow(t *testing.T) {
	db, _ := testutil.TempDB(t)
	now := storage.NowUTC()
	payload := map[string]any{"id": "t1", "title": "task", "status": "done", "created_at": now, "model_id": "test"}
	if err := archiveRow(db, "tasks", "t1", payload, now, "direct"); err != nil {
		t.Fatal(err)
	}
	if err := archiveRow(db, "not_a_table", "x", payload, now, "fail"); err == nil {
		t.Fatal("expected archive row error")
	}
}

func TestRunVacuumAndGCPaths(t *testing.T) {
	db, _ := testutil.TempDB(t)
	old := time.Now().UTC().Add(-72 * time.Hour).Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO reminders(id, title, status, created_at, model_id) VALUES ('r1','r','dismissed',?,'test')`, old); err != nil {
		t.Fatal(err)
	}
	res, err := Run(db, RunOptions{Yes: true, SessionHours: 24, Vacuum: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.ArchivedTotal < 0 {
		t.Fatalf("run=%+v", res)
	}
	gc, err := GC(db, GCOptions{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if gc.FeedbackToClose < 0 {
		t.Fatalf("gc=%+v", gc)
	}
}
