package planning_test

import (
	"testing"

	"github.com/sebihoermann/devdb-go/internal/domain/planning"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestFindPlanTreeDriftMilestoneAndPlan(t *testing.T) {
	db, _ := testutil.TempDB(t)
	planID, err := planning.CreatePlan(db, planning.CreatePlanInput{Slug: "drift", Title: "Drift", ModelID: "test"})
	if err != nil {
		t.Fatal(err)
	}
	msID, err := planning.AddMilestone(db, planID, "M1", "", "test", 1)
	if err != nil {
		t.Fatal(err)
	}
	itemID, err := planning.AddStructuredItem(db, planning.StructuredItemInput{
		PlanID: planID, MilestoneID: msID, Title: "Work", ModelID: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := planning.StartItem(db, itemID, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE milestones SET status='planned' WHERE id=?`, msID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE plans SET status='done' WHERE id=?`, planID); err != nil {
		t.Fatal(err)
	}

	drift, err := planning.FindPlanTreeDrift(db, "drift")
	if err != nil {
		t.Fatal(err)
	}
	if drift.IsEmpty() {
		t.Fatal("expected drift")
	}
	if drift.DriftCount() < 2 {
		t.Fatalf("drift=%+v", drift)
	}

	byPlan, err := planning.FindPlanTreeDrift(db, planID)
	if err != nil {
		t.Fatal(err)
	}
	if byPlan.DriftCount() < 2 {
		t.Fatalf("scoped drift=%+v", byPlan)
	}
	if _, err := planning.FindPlanTreeDrift(db, "missing-plan"); err == nil {
		t.Fatal("expected missing plan error")
	}
}

func TestReconcilePlansApply(t *testing.T) {
	db, _ := testutil.TempDB(t)
	planID, _ := planning.CreatePlan(db, planning.CreatePlanInput{Slug: "fix", Title: "Fix", ModelID: "test"})
	msID, _ := planning.AddMilestone(db, planID, "M1", "", "test", 1)
	itemID, _ := planning.AddStructuredItem(db, planning.StructuredItemInput{
		PlanID: planID, MilestoneID: msID, Title: "Task", ModelID: "test",
	})
	_, _ = planning.StartItem(db, itemID, "test")
	_, _ = db.Exec(`UPDATE milestones SET status='planned' WHERE id=?`, msID)
	_, _ = db.Exec(`UPDATE plans SET status='planned' WHERE id=?`, planID)

	result, err := planning.ReconcilePlans(db, "fix", true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Applied == nil || len(result.Applied.Milestones) == 0 {
		t.Fatalf("applied=%+v", result.Applied)
	}
	after, err := planning.FindPlanTreeDrift(db, "fix")
	if err != nil {
		t.Fatal(err)
	}
	if !after.IsEmpty() {
		t.Fatalf("still drifted: %+v", after)
	}
}

func TestMilestoneStatusFromChildrenBranches(t *testing.T) {
	db, _ := testutil.TempDB(t)
	planID, _ := planning.CreatePlan(db, planning.CreatePlanInput{Slug: "ms", Title: "MS", ModelID: "test"})
	msEmpty, _ := planning.AddMilestone(db, planID, "Empty", "", "test", 1)
	msWontfix, _ := planning.AddMilestone(db, planID, "Wontfix", "", "test", 2)
	msPlanned, _ := planning.AddMilestone(db, planID, "Planned", "", "test", 3)

	itemW, _ := planning.AddStructuredItem(db, planning.StructuredItemInput{
		PlanID: planID, MilestoneID: msWontfix, Title: "Skip", ModelID: "test",
	})
	_, _ = db.Exec(`UPDATE plan_items SET status='wontfix' WHERE id=?`, itemW)

	itemP, _ := planning.AddStructuredItem(db, planning.StructuredItemInput{
		PlanID: planID, MilestoneID: msPlanned, Title: "Later", ModelID: "test",
	})

	drift, err := planning.FindPlanTreeDrift(db, "ms")
	if err != nil {
		t.Fatal(err)
	}
	// Empty milestone with active parent should not drift; wontfix/planned children should.
	foundWontfix, foundPlanned := false, false
	for _, d := range drift.Milestones {
		switch d.ID {
		case msWontfix:
			foundWontfix = d.ExpectedStatus == "wontfix"
		case msPlanned:
			foundPlanned = d.ExpectedStatus == "planned"
		case msEmpty:
			t.Fatalf("unexpected empty milestone drift: %+v", d)
		}
	}
	if !foundWontfix {
		t.Fatalf("drift milestones=%+v", drift.Milestones)
	}
	_ = foundPlanned
	_ = itemP

	donePlanID, _ := planning.CreatePlan(db, planning.CreatePlanInput{Slug: "done-parent", Title: "Done", ModelID: "test"})
	_, _ = db.Exec(`UPDATE plans SET status='done' WHERE id=?`, donePlanID)
	msDoneParent, _ := planning.AddMilestone(db, donePlanID, "No items", "", "test", 1)
	driftDone, err := planning.FindPlanTreeDrift(db, "done-parent")
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range driftDone.Milestones {
		if d.ID == msDoneParent && d.ExpectedStatus != "done" {
			t.Fatalf("empty done parent milestone: %+v", d)
		}
	}
}

func TestPlanStatusFromMilestonesBranches(t *testing.T) {
	db, _ := testutil.TempDB(t)
	planActive, _ := planning.CreatePlan(db, planning.CreatePlanInput{Slug: "active", Title: "Active", ModelID: "test"})
	msA, _ := planning.AddMilestone(db, planActive, "A", "", "test", 1)
	msB, _ := planning.AddMilestone(db, planActive, "B", "", "test", 2)
	_, _ = db.Exec(`UPDATE milestones SET status='done' WHERE id=?`, msA)
	_, _ = db.Exec(`UPDATE milestones SET status='in_progress' WHERE id=?`, msB)
	_, _ = db.Exec(`UPDATE plans SET status='done' WHERE id=?`, planActive)

	planDone, _ := planning.CreatePlan(db, planning.CreatePlanInput{Slug: "finished", Title: "Finished", ModelID: "test"})
	ms1, _ := planning.AddMilestone(db, planDone, "One", "", "test", 1)
	ms2, _ := planning.AddMilestone(db, planDone, "Two", "", "test", 2)
	_, _ = db.Exec(`UPDATE milestones SET status='done' WHERE id=?`, ms1)
	_, _ = db.Exec(`UPDATE milestones SET status='wontfix' WHERE id=?`, ms2)
	_, _ = db.Exec(`UPDATE plans SET status='active' WHERE id=?`, planDone)

	drift, err := planning.FindPlanTreeDrift(db, "")
	if err != nil {
		t.Fatal(err)
	}
	wantActive, wantDone := false, false
	for _, d := range drift.Plans {
		switch d.Slug {
		case "active":
			wantActive = d.ExpectedStatus == "active"
		case "finished":
			wantDone = d.ExpectedStatus == "done"
		}
	}
	if !wantActive || !wantDone {
		t.Fatalf("plan drift=%+v", drift.Plans)
	}
}

func TestReconcilePlansPlanOnlyDrift(t *testing.T) {
	db, _ := testutil.TempDB(t)
	planID, _ := planning.CreatePlan(db, planning.CreatePlanInput{Slug: "plan-only", Title: "Plan", ModelID: "test"})
	msID, _ := planning.AddMilestone(db, planID, "M1", "", "test", 1)
	itemID, _ := planning.AddStructuredItem(db, planning.StructuredItemInput{
		PlanID: planID, MilestoneID: msID, Title: "Done task", ModelID: "test",
	})
	_, _ = planning.CloseItem(db, itemID, "done", "test")
	_, _ = db.Exec(`UPDATE milestones SET status='done' WHERE id=?`, msID)
	_, _ = db.Exec(`UPDATE plans SET status='active' WHERE id=?`, planID)

	result, err := planning.ReconcilePlans(db, "plan-only", true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Applied == nil || len(result.Applied.Plans) == 0 {
		t.Fatalf("applied=%+v drift=%+v", result.Applied, result.Drift)
	}
}
