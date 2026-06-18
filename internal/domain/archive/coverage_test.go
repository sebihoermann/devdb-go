package archive

import (
	"testing"
	"time"

	"github.com/sebihoermann/devdb-go/internal/domain/feedback"
	"github.com/sebihoermann/devdb-go/internal/domain/review"
)

func TestArchiveReviewFindingsResolved(t *testing.T) {
	db := archiveTestDB(t)
	runID, _ := review.StartRun(db, []string{"."}, "default", "", "test")
	findingID, _ := review.AddFinding(db, runID, review.FindingInput{
		FilePath: "z.go", Principle: "kiss", Title: "issue", Recommendation: "fix",
		Severity: "low", Confidence: "low", Effort: "trivial",
	}, "test")
	_, _ = review.ResolveFinding(db, findingID, "", "wontfix", "not fixing")
	res, err := Run(db, RunOptions{DryRun: true, Table: "review_findings"})
	if err != nil || res.ByTable["review_findings"] != 1 {
		t.Fatalf("findings dry-run: %+v err=%v", res, err)
	}
}

func TestGCStaleFindingAndArchNote(t *testing.T) {
	db := archiveTestDB(t)
	if _, err := db.Exec(`INSERT INTO repo_files(path, kind, content_hash, last_seen_at) VALUES ('gone.go','code','h',datetime('now')), ('keep.go','code','h',datetime('now'))`); err != nil {
		t.Fatal(err)
	}
	runID, _ := review.StartRun(db, []string{"."}, "default", "", "test")
	_, _ = review.AddFinding(db, runID, review.FindingInput{
		FilePath: "gone.go", Principle: "dry", Title: "missing file", Recommendation: "fix",
		Severity: "low", Confidence: "low", Effort: "trivial",
	}, "test")
	if _, err := db.Exec(`DELETE FROM repo_files WHERE path='gone.go'`); err != nil {
		t.Fatal(err)
	}

	old := time.Now().UTC().AddDate(0, 0, -40).Format(time.RFC3339Nano)
	id, _ := feedback.Add(db, feedback.AddInput{Role: "model", Note: "old open", ModelID: "test"})
	if _, err := db.Exec(`UPDATE feedback SET created_at=? WHERE id=?`, old, id); err != nil {
		t.Fatal(err)
	}

	res, err := GC(db, GCOptions{OlderThanDays: 30, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.FindingsToWontfix < 1 || res.FeedbackToClose < 1 {
		t.Fatalf("gc dry-run: %+v", res)
	}
	res, err = GC(db, GCOptions{OlderThanDays: 30})
	if err != nil {
		t.Fatal(err)
	}
	if res.FindingsResolved < 1 || res.FeedbackClosed < 1 {
		t.Fatalf("gc apply: %+v", res)
	}
}

func TestListArchiveEntriesFilter(t *testing.T) {
	db := archiveTestDB(t)
	old := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO features(id, title, created_at, model_id) VALUES ('fx','feat',?,'test')`, old); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(db, RunOptions{SessionHours: 24, Yes: true}); err != nil {
		t.Fatal(err)
	}
	entries, err := List(db, ListFilter{Table: "features", Limit: 5})
	if err != nil || len(entries) != 1 {
		t.Fatalf("list features archive: %d err=%v", len(entries), err)
	}
}
