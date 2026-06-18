package archive

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/sebihoermann/devdb-go/internal/domain/feedback"
	"github.com/sebihoermann/devdb-go/internal/domain/reminders"
	"github.com/sebihoermann/devdb-go/internal/domain/tasks"
	"github.com/sebihoermann/devdb-go/internal/storage"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func archiveTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, _ := testutil.TempDB(t)
	db.SetMaxOpenConns(2)
	return db
}

func TestArchiveOrphansDirect(t *testing.T) {
	db := archiveTestDB(t)
	if _, err := db.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatal(err)
	}
	now := storage.NowUTC()
	if _, err := db.Exec(`
		INSERT INTO plan_item_acceptance(id, plan_item_id, ordinal, criterion, status, created_at, updated_at, model_id)
		VALUES ('acc-orphan', 'gone-plan', 1, 'met', 'met', ?, ?, 'test')`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO plan_item_files(id, plan_item_id, path, role, created_at, model_id)
		VALUES ('file-orphan', 'gone-plan', 'orphan.go', 'modify', ?, 'test')`, now); err != nil {
		t.Fatal(err)
	}

	n, err := archiveOrphans(db, "plan_item_acceptance", now)
	if err != nil || n != 1 {
		t.Fatalf("acceptance orphans=%d err=%v", n, err)
	}
	n, err = archiveOrphans(db, "plan_item_files", now)
	if err != nil || n != 1 {
		t.Fatalf("files orphans=%d err=%v", n, err)
	}
	var left int
	if err := db.QueryRow(`SELECT COUNT(*) FROM plan_item_acceptance`).Scan(&left); err != nil || left != 0 {
		t.Fatalf("acceptance rows left=%d", left)
	}
}

func TestArchiveOrphansViaRun(t *testing.T) {
	db := archiveTestDB(t)
	if _, err := db.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatal(err)
	}
	now := storage.NowUTC()
	old := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339Nano)
	if _, err := db.Exec(`
		INSERT INTO plan_item_files(id, plan_item_id, path, role, created_at, model_id)
		VALUES ('f-orphan', 'missing-parent', 'x.go', 'modify', ?, 'test')`, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO features(id, title, created_at, model_id) VALUES ('f-old','old',?,'test')`, old); err != nil {
		t.Fatal(err)
	}
	res, err := Run(db, RunOptions{Yes: true, SessionHours: 24})
	if err != nil {
		t.Fatal(err)
	}
	if res.ByTable["plan_item_files"] != 1 {
		t.Fatalf("orphan archive via run: %+v", res)
	}
}

func TestCountAndArchiveLocSnapshots(t *testing.T) {
	db := archiveTestDB(t)
	if err := createLocSnapshotsTable(db); err != nil {
		t.Fatal(err)
	}
	snaps := []string{
		"2020-01-01T00:00:00Z",
		"2021-01-01T00:00:00Z",
		"2022-01-01T00:00:00Z",
		"2023-01-01T00:00:00Z",
	}
	for i, snap := range snaps {
		id, _ := storage.NewID()
		if _, err := db.Exec(
			`INSERT INTO loc_snapshots(id, snapshot_at, file_path, lines, created_at, model_id) VALUES (?, ?, ?, ?, ?, ?)`,
			id, snap, "pkg/a.go", 10+i, snap, "test",
		); err != nil {
			t.Fatal(err)
		}
	}

	wantDrop, err := countLocSnapshots(db, 2)
	if err != nil || wantDrop != 2 {
		t.Fatalf("countLocSnapshots=%d err=%v", wantDrop, err)
	}

	archivedAt := storage.NowUTC()
	bundled, err := archiveLocSnapshots(db, archivedAt, "retention", 2)
	if err != nil || bundled != 2 {
		t.Fatalf("archiveLocSnapshots bundled=%d err=%v", bundled, err)
	}
	var remain int
	if err := db.QueryRow(`SELECT COUNT(*) FROM loc_snapshots`).Scan(&remain); err != nil || remain != 2 {
		t.Fatalf("remaining snapshots=%d", remain)
	}

	entries, err := List(db, ListFilter{Table: "loc_snapshots"})
	if err != nil || len(entries) != 2 {
		t.Fatalf("archive entries=%d err=%v", len(entries), err)
	}

	restored, err := Restore(db, RestoreOptions{Table: "loc_snapshots"})
	if err != nil || restored.Restored != 2 {
		t.Fatalf("restore loc snapshots: %+v err=%v", restored, err)
	}
}

func TestRestoreLocSnapshotEdgeCases(t *testing.T) {
	db := archiveTestDB(t)
	if err := createLocSnapshotsTable(db); err != nil {
		t.Fatal(err)
	}
	snap := storage.NowUTC()
	payload := map[string]any{
		"snapshot_at": snap,
		"files":       map[string]any{"a.go": 12},
	}
	n, err := restoreLocSnapshot(db, payload)
	if err != nil || n != 1 {
		t.Fatalf("restore with int lines: n=%d err=%v", n, err)
	}

	payload["model_id"] = ""
	n, err = restoreLocSnapshot(db, payload)
	if err != nil || n != 1 {
		t.Fatalf("restore default model: n=%d err=%v", n, err)
	}

	if _, err := db.Exec(`DROP TABLE loc_snapshots`); err != nil {
		t.Fatal(err)
	}
	n, err = restoreLocSnapshot(db, payload)
	if err != nil || n != 0 {
		t.Fatalf("missing table: n=%d err=%v", n, err)
	}
}

func TestListArchiveSinceUntil(t *testing.T) {
	db := archiveTestDB(t)
	old := time.Now().UTC().Add(-72 * time.Hour).Format(time.RFC3339Nano)
	mid := time.Now().UTC().Add(-36 * time.Hour).Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO features(id, title, created_at, model_id) VALUES ('f-old','old',?,'test')`, old); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(db, RunOptions{SessionHours: 24, Yes: true}); err != nil {
		t.Fatal(err)
	}
	entries, err := List(db, ListFilter{Since: mid, Until: storage.NowUTC(), Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("expected entries in since/until window")
	}
}

func TestRestoreSkipAndKeepArchive(t *testing.T) {
	db := archiveTestDB(t)
	old := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO features(id, title, created_at, model_id) VALUES ('f1','feat',?,'test')`, old); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(db, RunOptions{SessionHours: 24, Yes: true}); err != nil {
		t.Fatal(err)
	}
	entries, err := List(db, ListFilter{Limit: 1})
	if err != nil || len(entries) != 1 {
		t.Fatal(err)
	}
	first, err := Restore(db, RestoreOptions{ID: entries[0].ID})
	if err != nil || first.Restored != 1 {
		t.Fatalf("first restore: %+v err=%v", first, err)
	}

	payloadJSON, _ := json.Marshal(map[string]any{"id": "f1", "title": "feat", "created_at": old, "model_id": "test"})
	dupID, _ := storage.NewID()
	if _, err := db.Exec(`
		INSERT INTO archive_entries(id, source_table, source_id, payload_json, archived_at, archive_reason)
		VALUES (?, 'features', 'f1', ?, ?, 'dup')`, dupID, string(payloadJSON), storage.NowUTC()); err != nil {
		t.Fatal(err)
	}
	second, err := Restore(db, RestoreOptions{ID: dupID})
	if err != nil || second.SkippedAlreadyPresent != 1 {
		t.Fatalf("second restore: %+v err=%v", second, err)
	}

	if _, err := db.Exec(`DELETE FROM features`); err != nil {
		t.Fatal(err)
	}
	archID, _ := storage.NewID()
	if _, err := db.Exec(`
		INSERT INTO archive_entries(id, source_table, source_id, payload_json, archived_at, archive_reason)
		VALUES (?, 'features', 'f1', ?, ?, 'test')`, archID, string(payloadJSON), storage.NowUTC()); err != nil {
		t.Fatal(err)
	}
	kept, err := Restore(db, RestoreOptions{ID: archID, KeepArchive: true})
	if err != nil || kept.Restored != 1 || kept.ArchiveEntriesDeleted != 0 {
		t.Fatalf("keep archive: %+v err=%v", kept, err)
	}
	var archCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM archive_entries WHERE id=?`, archID).Scan(&archCount); err != nil || archCount != 1 {
		t.Fatalf("archive row kept: %d", archCount)
	}
}

func TestRestoreRequiresSelector(t *testing.T) {
	db := archiveTestDB(t)
	_, err := Restore(db, RestoreOptions{})
	if err == nil {
		t.Fatal("expected selector error")
	}
}

func TestCountRowsLocSnapshotBundle(t *testing.T) {
	db := archiveTestDB(t)
	if err := createLocSnapshotsTable(db); err != nil {
		t.Fatal(err)
	}
	snapOld := "2020-01-01T00:00:00Z"
	snapNew := storage.NowUTC()
	for _, row := range []struct {
		id, snap string
	}{
		{"id-old", snapOld},
		{"id-new", snapNew},
	} {
		if _, err := db.Exec(
			`INSERT INTO loc_snapshots(id, snapshot_at, file_path, lines, created_at, model_id) VALUES (?, ?, ?, ?, ?, ?)`,
			row.id, row.snap, "main.go", 1, row.snap, "test",
		); err != nil {
			t.Fatal(err)
		}
	}
	n, err := countRows(db, "loc_snapshots", "", nil, "by_snapshot_at", 1)
	if err != nil || n != 1 {
		t.Fatalf("countRows bundle=%d err=%v", n, err)
	}
}

func TestRunDefaultRetentionOptions(t *testing.T) {
	db := archiveTestDB(t)
	res, err := Run(db, RunOptions{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.SessionHours != 24 || res.KeepSnapshots != 3 {
		t.Fatalf("defaults: %+v", res)
	}
}

func TestArchiveTableSnapshotBundle(t *testing.T) {
	db := archiveTestDB(t)
	if err := createLocSnapshotsTable(db); err != nil {
		t.Fatal(err)
	}
	for i, snap := range []string{"2020-01-01T00:00:00Z", "2021-01-01T00:00:00Z"} {
		id, _ := storage.NewID()
		if _, err := db.Exec(
			`INSERT INTO loc_snapshots(id, snapshot_at, file_path, lines, created_at, model_id) VALUES (?, ?, ?, ?, ?, ?)`,
			id, snap, "pkg/a.go", 5+i, snap, "test",
		); err != nil {
			t.Fatal(err)
		}
	}
	n, err := archiveTable(db, archiveStep{bundle: "by_snapshot_at", reason: "bundle"}, storage.NowUTC(), 1)
	if err != nil || n != 1 {
		t.Fatalf("archiveTable bundle=%d err=%v", n, err)
	}
}

func TestRestoreFilterSelectors(t *testing.T) {
	db := archiveTestDB(t)
	old := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO features(id, title, created_at, model_id) VALUES ('fx','feat',?,'test')`, old); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(db, RunOptions{SessionHours: 24, Yes: true}); err != nil {
		t.Fatal(err)
	}
	archAt := storage.NowUTC()
	restored, err := Restore(db, RestoreOptions{
		SourceTable: "features",
		SourceID:    "fx",
		Table:       "features",
		Since:       old,
		Until:       archAt,
	})
	if err != nil || restored.Restored != 1 {
		t.Fatalf("filtered restore: %+v err=%v", restored, err)
	}
}

func TestRestoreRejectsInvalidJSON(t *testing.T) {
	db := archiveTestDB(t)
	archID, _ := storage.NewID()
	if _, err := db.Exec(`
		INSERT INTO archive_entries(id, source_table, source_id, payload_json, archived_at)
		VALUES (?, 'features', 'x', 'not-json', ?)`, archID, storage.NowUTC()); err != nil {
		t.Fatal(err)
	}
	_, err := Restore(db, RestoreOptions{ID: archID})
	if err == nil {
		t.Fatal("expected json error")
	}
}

func TestGCCountsDismissedRemindersAndTasks(t *testing.T) {
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
	taskID, err := tasks.Add(db, "done", "", "med", "", "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tasks.SetStatus(db, taskID, "done", "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE tasks SET created_at=? WHERE id=?`, old, taskID); err != nil {
		t.Fatal(err)
	}
	res, err := GC(db, GCOptions{OlderThanDays: 30, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.RemindersToArchive < 1 || res.TasksToArchive < 1 {
		t.Fatalf("gc counts: %+v", res)
	}
}

func TestArchiveClosedFeedbackAndStatusLog(t *testing.T) {
	db := archiveTestDB(t)
	old := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339Nano)
	fbID, err := feedback.Add(db, feedback.AddInput{Role: "model", Note: "closed", ModelID: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE feedback SET status='closed', created_at=? WHERE id=?`, old, fbID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO status_log(id, plan_item_id, status, note, created_at, model_id)
		VALUES ('sl1', NULL, 'note', 'old log', ?, 'test')`, old); err != nil {
		t.Fatal(err)
	}
	res, err := Run(db, RunOptions{SessionHours: 24, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.ByTable["feedback"] < 1 || res.ByTable["status_log"] < 1 {
		t.Fatalf("dry-run counts: %+v", res)
	}
}

func TestArchiveSweepMultipleEntityTypes(t *testing.T) {
	db := archiveTestDB(t)
	old := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO features(id, title, created_at, model_id) VALUES ('f1','feat',?,'test')`, old); err != nil {
		t.Fatal(err)
	}
	fbID, err := feedback.Add(db, feedback.AddInput{Role: "model", Note: "closed", ModelID: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE feedback SET status='closed', created_at=? WHERE id=?`, old, fbID); err != nil {
		t.Fatal(err)
	}
	remID, err := reminders.Add(db, reminders.AddInput{Title: "r", ModelID: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reminders.Dismiss(db, remID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE reminders SET created_at=? WHERE id=?`, old, remID); err != nil {
		t.Fatal(err)
	}
	res, err := Run(db, RunOptions{SessionHours: 24, Yes: true, Vacuum: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.ArchivedTotal < 3 {
		t.Fatalf("sweep archived=%d want >=3: %+v", res.ArchivedTotal, res.ByTable)
	}
}

func TestCountLocSnapshotsEmptyTable(t *testing.T) {
	db := archiveTestDB(t)
	n, err := countLocSnapshots(db, 3)
	if err != nil || n != 0 {
		t.Fatalf("empty table count=%d err=%v", n, err)
	}
}

func TestArchiveLocSnapshotsMissingTable(t *testing.T) {
	db := archiveTestDB(t)
	n, err := archiveLocSnapshots(db, storage.NowUTC(), "missing", 3)
	if err != nil || n != 0 {
		t.Fatalf("missing table bundled=%d err=%v", n, err)
	}
}

func TestGCAppliesReminderAndTaskArchive(t *testing.T) {
	db := archiveTestDB(t)
	remID, err := reminders.Add(db, reminders.AddInput{Title: "gc", ModelID: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reminders.Dismiss(db, remID); err != nil {
		t.Fatal(err)
	}
	taskID, err := tasks.Add(db, "gc task", "", "med", "", "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tasks.SetStatus(db, taskID, "wontfix", "test"); err != nil {
		t.Fatal(err)
	}
	old := time.Now().UTC().AddDate(0, 0, -40).Format(time.RFC3339Nano)
	if _, err := db.Exec(`UPDATE reminders SET created_at=? WHERE id=?`, old, remID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE tasks SET created_at=? WHERE id=?`, old, taskID); err != nil {
		t.Fatal(err)
	}
	res, err := GC(db, GCOptions{OlderThanDays: 30})
	if err != nil {
		t.Fatal(err)
	}
	if res.RemindersArchived < 1 || res.TasksArchived < 1 {
		t.Fatalf("gc apply: %+v", res)
	}
}

func TestArchiveDismissedReminderAndDoneTask(t *testing.T) {
	db := archiveTestDB(t)
	old := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339Nano)
	remID, err := reminders.Add(db, reminders.AddInput{Title: "old", ModelID: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reminders.Dismiss(db, remID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE reminders SET created_at=? WHERE id=?`, old, remID); err != nil {
		t.Fatal(err)
	}
	taskID, err := tasks.Add(db, "done", "", "med", "", "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tasks.SetStatus(db, taskID, "done", "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE tasks SET created_at=? WHERE id=?`, old, taskID); err != nil {
		t.Fatal(err)
	}
	res, err := Run(db, RunOptions{SessionHours: 24, Yes: true, Table: "reminders"})
	if err != nil || res.ArchivedTotal < 1 {
		t.Fatalf("reminder archive: %+v err=%v", res, err)
	}
	res, err = Run(db, RunOptions{SessionHours: 24, Yes: true, Table: "tasks"})
	if err != nil || res.ArchivedTotal < 1 {
		t.Fatalf("task archive: %+v err=%v", res, err)
	}
}

func TestRestoreSortsPlanItemsBeforeFindings(t *testing.T) {
	db := archiveTestDB(t)
	now := storage.NowUTC()
	planPayload, _ := json.Marshal(map[string]any{
		"id": "pi1", "title": "item", "status": "done", "created_at": now, "model_id": "test",
	})
	findingPayload, _ := json.Marshal(map[string]any{
		"id": "rf1", "run_id": "rr1", "title": "finding", "status": "open",
		"created_at": now, "model_id": "test", "principle": "kiss", "recommendation": "fix",
		"severity": "low", "confidence": "low", "effort": "trivial",
	})
	archPlan, _ := storage.NewID()
	archFinding, _ := storage.NewID()
	if _, err := db.Exec(`
		INSERT INTO archive_entries(id, source_table, source_id, payload_json, archived_at)
		VALUES (?, 'plan_items', 'pi1', ?, ?)`, archPlan, string(planPayload), now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO archive_entries(id, source_table, source_id, payload_json, archived_at)
		VALUES (?, 'review_findings', 'rf1', ?, ?)`, archFinding, string(findingPayload), now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO review_runs(id, scope_paths, tier, started_at, model_id)
		VALUES ('rr1', '.', 'default', ?, 'test')`, now); err != nil {
		t.Fatal(err)
	}
	restored, err := Restore(db, RestoreOptions{Since: now})
	if err != nil {
		t.Fatal(err)
	}
	if restored.ByTableRestored["plan_items"] < 1 {
		t.Fatalf("restore order: %+v", restored)
	}
}
