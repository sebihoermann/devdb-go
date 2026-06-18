package archive

import (
	"testing"
	"time"

	"github.com/sebihoermann/devdb-go/internal/domain/feedback"
	"github.com/sebihoermann/devdb-go/internal/domain/planning"
	"github.com/sebihoermann/devdb-go/internal/storage"
)

func TestRunArchivesFeedbackStatusLogAndReview(t *testing.T) {
	db := archiveTestDB(t)
	old := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339Nano)
	fbID, _ := feedback.Add(db, feedback.AddInput{Role: "model", Note: "closed", ModelID: "test"})
	_, _ = db.Exec(`UPDATE feedback SET status='closed', created_at=? WHERE id=?`, old, fbID)
	planID, _ := planning.CreatePlan(db, planning.CreatePlanInput{Slug: "sl", Title: "SL", ModelID: "test"})
	itemID, _ := planning.AddItem(db, planning.AddItemInput{PlanID: planID, Title: "done", ModelID: "test"})
	_, _ = planning.SetItemStatusExplicit(db, itemID, "done", "shipped", "test")
	_, _ = db.Exec(`UPDATE plan_items SET created_at=? WHERE id=?`, old, itemID)
	now := storage.NowUTC()
	if _, err := db.Exec(`INSERT INTO status_log(id, plan_item_id, status, note, created_at, model_id) VALUES ('sl1',?, 'done','note',?, 'test')`, itemID, old); err != nil {
		t.Fatal(err)
	}
	_ = now
	res, err := Run(db, RunOptions{Yes: true, SessionHours: 24})
	if err != nil {
		t.Fatal(err)
	}
	if res.ArchivedTotal < 2 {
		t.Fatalf("run=%+v", res)
	}
	restored, err := Restore(db, RestoreOptions{Since: time.Now().UTC().Add(-72*time.Hour).Format(time.RFC3339Nano), KeepArchive: true})
	if err != nil {
		t.Fatal(err)
	}
	if restored.Restored < 1 {
		t.Fatalf("restore=%+v", restored)
	}
}
