package approval

import (
	"testing"

	"github.com/sebihoermann/devdb-go/internal/domain/planning"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestApprovalPlanItemFlow(t *testing.T) {
	db, _ := testutil.TempDB(t)
	planID, _ := planning.CreatePlan(db, planning.CreatePlanInput{Slug: "ap", Title: "AP", ModelID: "test"})
	itemID, _ := planning.AddItem(db, planning.AddItemInput{PlanID: planID, Title: "Item", ModelID: "test"})
	if _, err := Request(db, "plan_items", itemID, "review", "test"); err != nil {
		t.Fatal(err)
	}
	pending, err := ListPending(db)
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending=%d err=%v", len(pending), err)
	}
	if _, err := Approve(db, "plan_items", itemID, "ok", "test"); err != nil {
		t.Fatal(err)
	}
	logs, err := Log(db, 1)
	if err != nil || len(logs) != 1 {
		t.Fatalf("log limit=%d err=%v", len(logs), err)
	}
}
