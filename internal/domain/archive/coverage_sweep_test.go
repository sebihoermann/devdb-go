package archive

import (
	"database/sql"
	"testing"
	"time"

	"github.com/sebihoermann/devdb-go/internal/domain/feedback"
	"github.com/sebihoermann/devdb-go/internal/domain/review"
	"github.com/sebihoermann/devdb-go/internal/domain/tasks"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func archiveSweepDB(t *testing.T) *sql.DB {
	t.Helper()
	db, _ := testutil.TempDB(t)
	return db
}

func TestRunUnknownTableAndEmptyArchive(t *testing.T) {
	db := archiveSweepDB(t)
	if _, err := Run(db, RunOptions{Table: "nope_table", Yes: true}); err == nil {
		t.Fatal("unknown table")
	}
	res, err := Run(db, RunOptions{Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.ArchivedTotal != 0 {
		t.Fatalf("empty archive=%+v", res)
	}
}

func TestArchiveReviewFindingsAndRuns(t *testing.T) {
	db := archiveSweepDB(t)
	runID, _ := review.StartRun(db, []string{"."}, "default", "", "test")
	findingID, _ := review.AddFinding(db, runID, review.FindingInput{
		FilePath: "a.go", Principle: "kiss", Title: "t", Recommendation: "r",
		Severity: "low", Confidence: "low", Effort: "trivial",
	}, "test")
	_, _ = review.ResolveFinding(db, findingID, "", "resolved", "fixed")
	_, _ = review.FinishRun(db, runID, "done")
	res, err := Run(db, RunOptions{Yes: true, Table: "review_findings"})
	if err != nil {
		t.Fatal(err)
	}
	if res.ByTable["review_findings"] < 1 {
		t.Fatalf("findings=%+v", res)
	}
	res2, err := Run(db, RunOptions{Yes: true, Table: "review_runs"})
	if err != nil {
		t.Fatal(err)
	}
	if res2.ByTable["review_runs"] < 1 {
		t.Fatalf("runs=%+v", res2)
	}
}

func TestGCExecuteNotDryRun(t *testing.T) {
	db := archiveSweepDB(t)
	old := time.Now().UTC().AddDate(0, 0, -45).Format(time.RFC3339Nano)
	fbID, _ := feedback.Add(db, feedback.AddInput{Role: "model", Note: "stale open", ModelID: "test"})
	_, _ = db.Exec(`UPDATE feedback SET created_at=? WHERE id=?`, old, fbID)
	res, err := GC(db, GCOptions{OlderThanDays: 30, DryRun: false})
	if err != nil {
		t.Fatal(err)
	}
	if res.FeedbackClosed < 1 {
		t.Fatalf("gc=%+v", res)
	}
}

func TestArchiveTasksAndPlanItems(t *testing.T) {
	db := archiveSweepDB(t)
	old := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339Nano)
	taskID, _ := tasks.Add(db, "done task", "", "med", "", "test")
	_, _ = tasks.SetStatus(db, taskID, "done", "test")
	_, _ = db.Exec(`UPDATE tasks SET created_at=? WHERE id=?`, old, taskID)
	if _, err := db.Exec(`INSERT INTO plan_items(id, title, status, created_at, model_id) VALUES ('pi1','done item','done',?,'test')`, old); err != nil {
		t.Fatal(err)
	}
	res, err := Run(db, RunOptions{SessionHours: 24, Yes: true, Vacuum: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.ArchivedTotal < 2 {
		t.Fatalf("archive=%+v", res)
	}
}

func TestListArchiveByTableAndLimit(t *testing.T) {
	db := archiveSweepDB(t)
	old := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO features(id, title, created_at, model_id) VALUES ('f-list','feat',?,'test')`, old); err != nil {
		t.Fatal(err)
	}
	_, _ = Run(db, RunOptions{SessionHours: 24, Yes: true})
	entries, err := List(db, ListFilter{Table: "features", Limit: 5})
	if err != nil || len(entries) == 0 {
		t.Fatalf("list=%d err=%v", len(entries), err)
	}
}
