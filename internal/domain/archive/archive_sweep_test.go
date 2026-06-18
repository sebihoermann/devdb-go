package archive

import (
	"testing"
	"time"

	"github.com/sebihoermann/devdb-go/internal/domain/feedback"
	"github.com/sebihoermann/devdb-go/internal/domain/planning"
	"github.com/sebihoermann/devdb-go/internal/domain/reminders"
	"github.com/sebihoermann/devdb-go/internal/domain/review"
	"github.com/sebihoermann/devdb-go/internal/domain/tasks"
)

func TestArchiveEveryRunTable(t *testing.T) {
	db := archiveTestDB(t)
	old := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339Nano)

	if _, err := db.Exec(`INSERT INTO features(id, title, created_at, model_id) VALUES ('feat','f',?,'test')`, old); err != nil {
		t.Fatal(err)
	}
	fbID, err := feedback.Add(db, feedback.AddInput{Role: "model", Note: "closed", ModelID: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE feedback SET status='closed', created_at=? WHERE id=?`, old, fbID); err != nil {
		t.Fatal(err)
	}
	runID, err := review.StartRun(db, []string{"."}, "default", "", "test")
	if err != nil {
		t.Fatal(err)
	}
	findingID, err := review.AddFinding(db, runID, review.FindingInput{
		FilePath: "a.go", Principle: "kiss", Title: "done", Recommendation: "fix",
		Severity: "low", Confidence: "low", Effort: "trivial",
	}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := review.ResolveFinding(db, findingID, "", "resolved", "fixed"); err != nil {
		t.Fatal(err)
	}
	if _, err := review.FinishRun(db, runID, "ok"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO status_log(id, plan_item_id, status, note, created_at, model_id)
		VALUES ('sl-old', NULL, 'note', 'old', ?, 'test')`, old); err != nil {
		t.Fatal(err)
	}
	planID, err := planning.CreatePlan(db, planning.CreatePlanInput{Slug: "arch", Title: "Arch", ModelID: "test"})
	if err != nil {
		t.Fatal(err)
	}
	itemID, err := planning.AddItem(db, planning.AddItemInput{PlanID: planID, Title: "done", ModelID: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := planning.SetItemStatusExplicit(db, itemID, "done", "ok", "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE plan_items SET created_at=? WHERE id=?`, old, itemID); err != nil {
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
	taskID, err := tasks.Add(db, "t", "", "med", "", "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tasks.SetStatus(db, taskID, "done", "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE tasks SET created_at=? WHERE id=?`, old, taskID); err != nil {
		t.Fatal(err)
	}

	res, err := Run(db, RunOptions{SessionHours: 24, Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.ArchivedTotal < 6 {
		t.Fatalf("archived=%d: %+v", res.ArchivedTotal, res.ByTable)
	}
}
