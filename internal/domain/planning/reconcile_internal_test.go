package planning

import (
	"testing"

	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestMilestoneStatusFromChildrenEmptyMilestone(t *testing.T) {
	db, _ := testutil.TempDB(t)
	planID, _ := CreatePlan(db, CreatePlanInput{Slug: "empty", Title: "Empty", ModelID: "test"})
	msID, _ := AddMilestone(db, planID, "Lonely", "", "test", 1)
	status, err := milestoneStatusFromChildren(db, msID)
	if err != nil || status != "" {
		t.Fatalf("status=%q err=%v", status, err)
	}
}

func TestPlanStatusFromMilestonesEmptyPlan(t *testing.T) {
	db, _ := testutil.TempDB(t)
	planID, _ := CreatePlan(db, CreatePlanInput{Slug: "nop", Title: "Nop", ModelID: "test"})
	status, err := planStatusFromMilestones(db, planID)
	if err != nil || status != "" {
		t.Fatalf("status=%q err=%v", status, err)
	}
}

func TestSyncMilestoneStatusNoop(t *testing.T) {
	db, _ := testutil.TempDB(t)
	planID, _ := CreatePlan(db, CreatePlanInput{Slug: "noop", Title: "Noop", ModelID: "test"})
	msID, _ := AddMilestone(db, planID, "M1", "", "test", 1)
	status, err := syncMilestoneStatus(db, msID)
	if err != nil || status != "" {
		t.Fatalf("status=%q err=%v", status, err)
	}
}

func TestSyncPlanForMilestoneAndStatus(t *testing.T) {
	db, _ := testutil.TempDB(t)
	planID, _ := CreatePlan(db, CreatePlanInput{Slug: "sync", Title: "Sync", ModelID: "test"})
	msID, _ := AddMilestone(db, planID, "M1", "", "test", 1)
	itemID, _ := AddStructuredItem(db, StructuredItemInput{PlanID: planID, MilestoneID: msID, Title: "Task", ModelID: "test"})
	_, _ = StartItem(db, itemID, "test")
	_, _ = db.Exec(`UPDATE milestones SET status='planned' WHERE id=?`, msID)
	_, _ = db.Exec(`UPDATE plans SET status='planned' WHERE id=?`, planID)
	status, err := syncMilestoneStatus(db, msID)
	if err != nil || status != "in_progress" {
		t.Fatalf("milestone status=%q err=%v", status, err)
	}
	planStatus, err := syncPlanForMilestone(db, msID)
	if err != nil || planStatus != "active" {
		t.Fatalf("plan status=%q err=%v", planStatus, err)
	}
	direct, err := syncPlanStatus(db, planID)
	if err != nil || direct != "active" {
		t.Fatalf("direct plan status=%q err=%v", direct, err)
	}
}
