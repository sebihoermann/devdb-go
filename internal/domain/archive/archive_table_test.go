package archive

import (
	"testing"
	"time"

	"github.com/sebihoermann/devdb-go/internal/domain/planning"
	"github.com/sebihoermann/devdb-go/internal/domain/review"
	"github.com/sebihoermann/devdb-go/internal/storage"
)

func TestArchiveTableDirect(t *testing.T) {
	db := archiveTestDB(t)
	old := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO features(id, title, created_at, model_id) VALUES ('ft','title',?,'test')`, old); err != nil {
		t.Fatal(err)
	}
	n, err := archiveTable(db, archiveStep{
		table: "features", where: "id=?", params: []any{"ft"}, reason: "direct",
	}, storage.NowUTC(), 3)
	if err != nil || n != 1 {
		t.Fatalf("archiveTable=%d err=%v", n, err)
	}
}

func TestArchivePlanItemChildrenDirect(t *testing.T) {
	db := archiveTestDB(t)
	planID, err := planning.CreatePlan(db, planning.CreatePlanInput{Slug: "p", Title: "P", ModelID: "test"})
	if err != nil {
		t.Fatal(err)
	}
	itemID, err := planning.AddItem(db, planning.AddItemInput{PlanID: planID, Title: "child", ModelID: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := planning.AddAcceptance(db, itemID, "met", "test", 1); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := archivePlanItemChildren(tx, []string{itemID}, storage.NowUTC()); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM plan_item_acceptance WHERE plan_item_id=?`, itemID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("children archived: %d", n)
	}
}

func TestReadTableRowsDirect(t *testing.T) {
	db := archiveTestDB(t)
	if _, err := db.Exec(`
		INSERT INTO feedback(id, role, note, created_at, model_id) VALUES ('rf','model','note',?,'test')`,
		storage.NowUTC(),
	); err != nil {
		t.Fatal(err)
	}
	rows, err := readTableRows(db, `SELECT * FROM feedback WHERE id=?`, "rf")
	if err != nil || len(rows) != 1 {
		t.Fatalf("readTableRows=%d err=%v", len(rows), err)
	}
	if rows[0].payload["note"] == nil {
		t.Fatal("expected payload note")
	}
}

func TestReadTableRowsInvalidQuery(t *testing.T) {
	db := archiveTestDB(t)
	if _, err := readTableRows(db, "SELECT FROM not_valid"); err == nil {
		t.Fatal("expected query error")
	}
}

func TestArchiveRowDirect(t *testing.T) {
	db := archiveTestDB(t)
	now := storage.NowUTC()
	if _, err := db.Exec(`
		INSERT INTO tasks(id, title, status, priority, created_at, model_id) VALUES ('t1','task','done','med',?,'test')`,
		now,
	); err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{"id": "t1", "title": "task", "status": "done", "priority": "med", "created_at": now, "model_id": "test"}
	if err := archiveRow(db, "tasks", "t1", payload, now, "direct"); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM tasks`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("task removed: %d", n)
	}
}

func TestArchiveRowDeleteFailure(t *testing.T) {
	db := archiveTestDB(t)
	now := storage.NowUTC()
	payload := map[string]any{"id": "x", "title": "t", "status": "done", "priority": "med", "created_at": now, "model_id": "test"}
	if err := archiveRow(db, "not_a_table", "x", payload, now, "fail"); err == nil {
		t.Fatal("expected delete failure")
	}
}

func TestArchivePlanItemChildrenQueryFailure(t *testing.T) {
	db := archiveTestDB(t)
	planID, err := planning.CreatePlan(db, planning.CreatePlanInput{Slug: "px", Title: "PX", ModelID: "test"})
	if err != nil {
		t.Fatal(err)
	}
	itemID, err := planning.AddItem(db, planning.AddItemInput{PlanID: planID, Title: "child", ModelID: "test"})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`DROP TABLE plan_item_acceptance`); err != nil {
		t.Fatal(err)
	}
	if err := archivePlanItemChildren(tx, []string{itemID}, storage.NowUTC()); err == nil {
		t.Fatal("expected child archive query failure")
	}
	_ = tx.Rollback()
}

func TestArchivePlanItemChildrenIncludesFilesAndStatusLog(t *testing.T) {
	db := archiveTestDB(t)
	planID, err := planning.CreatePlan(db, planning.CreatePlanInput{Slug: "p2", Title: "P2", ModelID: "test"})
	if err != nil {
		t.Fatal(err)
	}
	itemID, err := planning.AddItem(db, planning.AddItemInput{PlanID: planID, Title: "item", ModelID: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := planning.AddPlanFile(db, itemID, "main.go", "modify", "test"); err != nil {
		t.Fatal(err)
	}
	now := storage.NowUTC()
	if _, err := db.Exec(`
		INSERT INTO status_log(id, plan_item_id, status, note, created_at, model_id)
		VALUES ('sl-child', ?, 'done', 'note', ?, 'test')`, itemID, now); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := archivePlanItemChildren(tx, []string{itemID}, now); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"plan_item_files", "status_log"} {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil || n != 0 {
			t.Fatalf("%s should be archived: %d err=%v", table, n, err)
		}
	}
}

func TestGCDefaultOlderThanDays(t *testing.T) {
	db := archiveTestDB(t)
	res, err := GC(db, GCOptions{DryRun: true})
	if err != nil || res.OlderThanDays != 30 {
		t.Fatalf("default gc days: %+v err=%v", res, err)
	}
}

func TestRunEmptyDatabaseReturnsZero(t *testing.T) {
	db := archiveTestDB(t)
	res, err := Run(db, RunOptions{Yes: true})
	if err != nil || res.ArchivedTotal != 0 {
		t.Fatalf("empty run: %+v err=%v", res, err)
	}
}

func TestRunSkipsFinishedRunWithOpenFinding(t *testing.T) {
	db := archiveTestDB(t)
	runID, err := review.StartRun(db, []string{"."}, "default", "", "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := review.AddFinding(db, runID, review.FindingInput{
		FilePath: "a.go", Principle: "kiss", Title: "open", Recommendation: "fix",
		Severity: "low", Confidence: "low", Effort: "trivial",
	}, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := review.FinishRun(db, runID, "done"); err != nil {
		t.Fatal(err)
	}
	res, err := Run(db, RunOptions{DryRun: true, Table: "review_runs"})
	if err != nil {
		t.Fatal(err)
	}
	if res.ByTable["review_runs"] != 0 {
		t.Fatalf("run with open finding should not archive: %+v", res)
	}
}
