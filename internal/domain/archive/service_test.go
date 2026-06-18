package archive

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/sebihoermann/devdb-go/internal/domain/feedback"
	"github.com/sebihoermann/devdb-go/internal/domain/planning"
	"github.com/sebihoermann/devdb-go/internal/domain/reminders"
	"github.com/sebihoermann/devdb-go/internal/domain/review"
	"github.com/sebihoermann/devdb-go/internal/domain/tasks"
	"github.com/sebihoermann/devdb-go/internal/storage"
)

func TestArchiveRunAndRestore(t *testing.T) {
	db := archiveTestDB(t)
	old := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339Nano)
	_, err := db.Exec(`
		INSERT INTO features(id, title, created_at, model_id) VALUES ('f1', 'old feature', ?, 'test')`,
		old,
	)
	if err != nil {
		t.Fatal(err)
	}

	res, err := Run(db, RunOptions{SessionHours: 24, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.ByTable["features"] != 1 {
		t.Fatalf("dry-run features=%d want 1", res.ByTable["features"])
	}

	res, err = Run(db, RunOptions{SessionHours: 24, Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.ArchivedTotal != 1 {
		t.Fatalf("archived=%d want 1", res.ArchivedTotal)
	}

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM features`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("features should be empty, got %d err=%v", n, err)
	}

	entries, err := List(db, ListFilter{Limit: 10})
	if err != nil || len(entries) != 1 {
		t.Fatalf("archive list: %d entries err=%v", len(entries), err)
	}

	restored, err := Restore(db, RestoreOptions{ID: entries[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	if restored.Restored != 1 {
		t.Fatalf("restored=%d want 1", restored.Restored)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM features`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("feature back in table: %d", n)
	}
}

func TestGCClosesOldFeedback(t *testing.T) {
	db := archiveTestDB(t)
	id, err := feedback.Add(db, feedback.AddInput{
		Role: "model", Note: "stale", ModelID: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().UTC().AddDate(0, 0, -40).Format(time.RFC3339Nano)
	if _, err := db.Exec(`UPDATE feedback SET created_at=? WHERE id=?`, old, id); err != nil {
		t.Fatal(err)
	}

	res, err := GC(db, GCOptions{OlderThanDays: 30, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.FeedbackToClose != 1 {
		t.Fatalf("want 1 feedback candidate, got %d", res.FeedbackToClose)
	}

	res, err = GC(db, GCOptions{OlderThanDays: 30})
	if err != nil {
		t.Fatal(err)
	}
	if res.FeedbackClosed != 1 {
		t.Fatalf("closed=%d want 1", res.FeedbackClosed)
	}
	var status string
	if err := db.QueryRow(`SELECT status FROM feedback WHERE id=?`, id).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "closed" {
		t.Fatalf("status=%s want closed", status)
	}
}

func TestGCArchivesOldTask(t *testing.T) {
	db := archiveTestDB(t)
	id, err := tasks.Add(db, "done task", "", "med", "", "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tasks.SetStatus(db, id, "done", "test"); err != nil {
		t.Fatal(err)
	}
	old := time.Now().UTC().AddDate(0, 0, -40).Format(time.RFC3339Nano)
	if _, err := db.Exec(`UPDATE tasks SET created_at=? WHERE id=?`, old, id); err != nil {
		t.Fatal(err)
	}

	res, err := GC(db, GCOptions{OlderThanDays: 30})
	if err != nil {
		t.Fatal(err)
	}
	if res.TasksArchived != 1 {
		t.Fatalf("tasks archived=%d want 1", res.TasksArchived)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM tasks`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("task row should be gone")
	}
	var arch int
	if err := db.QueryRow(`SELECT COUNT(*) FROM archive_entries WHERE source_table='tasks'`).Scan(&arch); err != nil || arch != 1 {
		t.Fatalf("archive entry missing")
	}
}

func TestGCArchivesDismissedReminder(t *testing.T) {
	db := archiveTestDB(t)
	id, err := reminders.Add(db, reminders.AddInput{Title: "old", ModelID: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reminders.Dismiss(db, id); err != nil {
		t.Fatal(err)
	}
	old := time.Now().UTC().AddDate(0, 0, -40).Format(time.RFC3339Nano)
	if _, err := db.Exec(`UPDATE reminders SET created_at=? WHERE id=?`, old, id); err != nil {
		t.Fatal(err)
	}

	res, err := GC(db, GCOptions{OlderThanDays: 30})
	if err != nil {
		t.Fatal(err)
	}
	if res.RemindersArchived != 1 {
		t.Fatalf("reminders archived=%d want 1", res.RemindersArchived)
	}
}

func TestArchiveUnknownTable(t *testing.T) {
	db := archiveTestDB(t)
	_, err := Run(db, RunOptions{Table: "nope", DryRun: true})
	if err == nil {
		t.Fatal("expected error for unknown table")
	}
}

func TestArchivePlanItemWithChildren(t *testing.T) {
	db := archiveTestDB(t)
	old := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339Nano)

	planID, err := planning.CreatePlan(db, planning.CreatePlanInput{Slug: "p", Title: "P", ModelID: "test"})
	if err != nil {
		t.Fatal(err)
	}
	itemID, err := planning.AddItem(db, planning.AddItemInput{PlanID: planID, Title: "done item", ModelID: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := planning.AddAcceptance(db, itemID, "met", "test", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := planning.AddPlanFile(db, itemID, "main.go", "modify", "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := planning.SetItemStatusExplicit(db, itemID, "done", "finished", "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE plan_items SET created_at=? WHERE id=?`, old, itemID); err != nil {
		t.Fatal(err)
	}

	res, err := Run(db, RunOptions{SessionHours: 24, Yes: true, Table: "plan_items"})
	if err != nil {
		t.Fatal(err)
	}
	if res.ArchivedTotal < 1 {
		t.Fatalf("archived=%d", res.ArchivedTotal)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM plan_items`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("plan_items should be empty: %d", n)
	}
}

func TestRestoreLocSnapshotBundle(t *testing.T) {
	db := archiveTestDB(t)
	if err := createLocSnapshotsTable(db); err != nil {
		t.Fatal(err)
	}
	snap := storage.NowUTC()
	payload := map[string]any{
		"snapshot_at": snap,
		"model_id":    "test",
		"files":       map[string]any{"pkg/main.go": float64(42)},
	}
	payloadJSON, _ := json.Marshal(payload)
	archID, _ := storage.NewID()
	if _, err := db.Exec(`
		INSERT INTO archive_entries(id, source_table, source_id, payload_json, archived_at, archive_reason)
		VALUES (?, 'loc_snapshots', ?, ?, ?, 'test')`,
		archID, snap, string(payloadJSON), snap); err != nil {
		t.Fatal(err)
	}
	restored, err := Restore(db, RestoreOptions{ID: archID})
	if err != nil {
		t.Fatal(err)
	}
	if restored.Restored < 1 {
		t.Fatalf("restored=%d", restored.Restored)
	}
}

func createLocSnapshotsTable(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS loc_snapshots (
		id TEXT PRIMARY KEY,
		snapshot_at TEXT NOT NULL,
		file_path TEXT NOT NULL,
		lines INTEGER,
		created_at TEXT,
		model_id TEXT
	)`)
	return err
}

func TestArchiveReviewRunFinished(t *testing.T) {
	db := archiveTestDB(t)
	runID, err := review.StartRun(db, []string{"."}, "default", "", "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := review.FinishRun(db, runID, "clean"); err != nil {
		t.Fatal(err)
	}
	res, err := Run(db, RunOptions{DryRun: true, Table: "review_runs"})
	if err != nil {
		t.Fatal(err)
	}
	if res.ByTable["review_runs"] != 1 {
		t.Fatalf("review_runs=%d", res.ByTable["review_runs"])
	}
}

func TestArchiveVacuum(t *testing.T) {
	db := archiveTestDB(t)
	old := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339Nano)
	_, err := db.Exec(`INSERT INTO features(id, title, created_at, model_id) VALUES ('f2','old',?,'test')`, old)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Run(db, RunOptions{SessionHours: 24, Yes: true, Vacuum: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.ArchivedTotal < 1 {
		t.Fatalf("archived=%d", res.ArchivedTotal)
	}
}

// Ensure storage.NowUTC is used at package init.
var _ = storage.NowUTC
