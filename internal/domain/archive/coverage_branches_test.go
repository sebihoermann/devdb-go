package archive

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/sebihoermann/devdb-go/internal/domain/planning"
	"github.com/sebihoermann/devdb-go/internal/domain/review"
	"github.com/sebihoermann/devdb-go/internal/storage"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestRunDryRunAndListFilters(t *testing.T) {
	db, _ := testutil.TempDB(t)
	old := time.Now().UTC().Add(-72 * time.Hour).Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO features(id, title, created_at, model_id) VALUES ('f-dry','feat',?,'test')`, old); err != nil {
		t.Fatal(err)
	}
	dry, err := Run(db, RunOptions{DryRun: true, SessionHours: 24})
	if err != nil || dry.WouldArchiveTotal < 1 || !dry.DryRun {
		t.Fatalf("dry=%+v err=%v", dry, err)
	}
	_, _ = Run(db, RunOptions{Yes: true, SessionHours: 24})
	since := time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339Nano)
	until := time.Now().UTC().Add(1 * time.Hour).Format(time.RFC3339Nano)
	entries, err := List(db, ListFilter{Table: "features", Since: since, Until: until, Limit: 10})
	if err != nil || len(entries) == 0 {
		t.Fatalf("list=%d err=%v", len(entries), err)
	}
}

func TestRestoreFeatureAndKeepArchive(t *testing.T) {
	db, _ := testutil.TempDB(t)
	old := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO features(id, title, created_at, model_id) VALUES ('f-restore','feat',?,'test')`, old); err != nil {
		t.Fatal(err)
	}
	_, _ = Run(db, RunOptions{Yes: true, Table: "features", SessionHours: 24})
	entries, _ := List(db, ListFilter{Table: "features"})
	if len(entries) == 0 {
		t.Fatal("no archived feature")
	}
	res, err := Restore(db, RestoreOptions{ID: entries[0].ID, KeepArchive: true})
	if err != nil || res.Restored != 1 {
		t.Fatalf("restore keep=%+v err=%v", res, err)
	}
	res2, err := Restore(db, RestoreOptions{ID: entries[0].ID, KeepArchive: true})
	if err != nil {
		t.Fatal(err)
	}
	if res2.SkippedAlreadyPresent < 1 {
		t.Fatalf("skip=%+v", res2)
	}
	if _, err := Restore(db, RestoreOptions{}); err == nil {
		t.Fatal("expected selector error")
	}
}

func TestRestoreLocSnapshotPayload(t *testing.T) {
	db, _ := testutil.TempDB(t)
	if err := createLocSnapshotsTable(db); err != nil {
		t.Fatal(err)
	}
	now := storage.NowUTC()
	payload := map[string]any{
		"snapshot_at": now,
		"files":       map[string]any{"pkg/a.go": float64(42)},
		"model_id":    "test",
	}
	raw, _ := json.Marshal(payload)
	archID, _ := storage.NewID()
	if _, err := db.Exec(`
		INSERT INTO archive_entries(id, source_table, source_id, payload_json, archived_at, archive_reason)
		VALUES (?, 'loc_snapshots', 'bundle', ?, ?, 'test')`,
		archID, string(raw), now); err != nil {
		t.Fatal(err)
	}
	res, err := Restore(db, RestoreOptions{ID: archID})
	if err != nil || res.Restored != 1 || res.ByTableRestored["loc_snapshots"] != 1 {
		t.Fatalf("loc restore=%+v err=%v", res, err)
	}
}

func TestArchivePlanItemChildrenAndRestoreOrder(t *testing.T) {
	db, _ := testutil.TempDB(t)
	old := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339Nano)
	planID, _ := planning.CreatePlan(db, planning.CreatePlanInput{Slug: "arch", Title: "Arch", ModelID: "test"})
	msID, _ := planning.AddMilestone(db, planID, "M1", "", "test", 1)
	itemID, _ := planning.AddStructuredItem(db, planning.StructuredItemInput{
		PlanID: planID, MilestoneID: msID, Title: "Item", ModelID: "test",
	})
	_, _ = planning.AddAcceptance(db, itemID, "done", "test", 1)
	_, _ = db.Exec(`UPDATE plan_items SET status='done', created_at=? WHERE id=?`, old, itemID)
	runID, _ := review.StartRun(db, []string{"."}, "default", "", "test")
	findingID, _ := review.AddFinding(db, runID, review.FindingInput{
		FilePath: "x.go", Principle: "kiss", Title: "t", Recommendation: "r",
		Severity: "low", Confidence: "low", Effort: "trivial",
	}, "test")
	_, _ = review.ResolveFinding(db, findingID, "", "resolved", "ok")
	_, _ = review.FinishRun(db, runID, "done")

	_, _ = Run(db, RunOptions{Yes: true, SessionHours: 24})
	restored, err := Restore(db, RestoreOptions{Table: "plan_items"})
	if err != nil {
		t.Fatal(err)
	}
	if restored.Restored < 1 {
		t.Fatalf("restore order=%+v", restored)
	}
}

func TestGCApplyFindingsRemindersTasks(t *testing.T) {
	db, _ := testutil.TempDB(t)
	old := time.Now().UTC().AddDate(0, 0, -45).Format(time.RFC3339Nano)
	now := storage.NowUTC()
	if _, err := db.Exec(`INSERT INTO repo_files(path, kind, lines, content_hash, last_seen_at) VALUES ('real.go','go',1,'h',?)`, now); err != nil {
		t.Fatal(err)
	}
	runID, _ := review.StartRun(db, []string{"."}, "default", "", "test")
	findingID, _ := review.AddFinding(db, runID, review.FindingInput{
		FilePath: "ghost.go", Principle: "kiss", Title: "ghost", Recommendation: "r",
		Severity: "high", Confidence: "low", Effort: "trivial",
	}, "test")
	_ = findingID
	if _, err := db.Exec(`INSERT INTO reminders(id, title, status, created_at, model_id) VALUES ('r-gc','r','dismissed',?,'test')`, old); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO tasks(id, title, status, created_at, model_id) VALUES ('t-gc','t','done',?,'test')`, old); err != nil {
		t.Fatal(err)
	}
	res, err := GC(db, GCOptions{OlderThanDays: 30, DryRun: false})
	if err != nil {
		t.Fatal(err)
	}
	if res.FindingsResolved < 1 || res.RemindersArchived < 1 || res.TasksArchived < 1 {
		t.Fatalf("gc apply=%+v", res)
	}
}

func TestArchiveLocSnapshotsDirect(t *testing.T) {
	db := archiveTestDB(t)
	if err := createLocSnapshotsTable(db); err != nil {
		t.Fatal(err)
	}
	for i, snap := range []string{"2020-01-01T00:00:00Z", "2021-01-01T00:00:00Z", "2022-01-01T00:00:00Z"} {
		id, _ := storage.NewID()
		if _, err := db.Exec(
			`INSERT INTO loc_snapshots(id, snapshot_at, file_path, lines, created_at, model_id) VALUES (?, ?, ?, ?, ?, ?)`,
			id, snap, "a.go", 10+i, snap, "test",
		); err != nil {
			t.Fatal(err)
		}
	}
	now := storage.NowUTC()
	n, err := archiveLocSnapshots(db, now, "retention", 1)
	if err != nil || n < 1 {
		t.Fatalf("loc archive=%d err=%v", n, err)
	}
}
