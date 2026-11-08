package planning_test

import (
	"database/sql"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/domain/archive"
	"github.com/sebihoermann/devdb-go/internal/domain/planning"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

// buildOneItemPlan creates the smallest plan that exercises the tree:
// one plan, one milestone, one item, two criteria — all complete.
func buildOneItemPlan(t *testing.T, db *sql.DB, slug string) (planID, msID, itemID string) {
	t.Helper()

	planID, err := planning.CreatePlan(db, planning.CreatePlanInput{
		Slug: slug, Title: "archived reconcile plan", Body: "", ModelID: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	msID, err = planning.AddMilestone(db, planID, "M1", "", "test", 0)
	if err != nil {
		t.Fatal(err)
	}
	itemID, err = planning.AddItem(db, planning.AddItemInput{
		PlanID: planID, MilestoneID: msID, Title: "the item", Body: "", ModelID: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	for i, criterion := range []string{"first", "second"} {
		if _, err := planning.AddAcceptance(db, itemID, criterion, "test", i+1); err != nil {
			t.Fatal(err)
		}
	}
	// The sweep only takes rows created before the session cutoff, so backdate
	// the fixture into "pre-session" history.
	if _, err := db.Exec(`UPDATE plan_items SET created_at='2020-01-01T00:00:00Z' WHERE id=?`, itemID); err != nil {
		t.Fatal(err)
	}
	return planID, msID, itemID
}

// TestReconcileCountsArchivedChildren is the documented repro: a plan whose
// items are all archived after completion must still reconcile to done.
//
// Before this, milestoneStatusFromChildren counted live plan_items only. After
// `archive run` moved the finished items into archive_entries the query
// returned zero rows, so reconcile computed no expected status, reported "no
// drift", and the plan stayed active forever with zero live children. The only
// escape was a direct UPDATE — exactly the state reconcile exists to prevent.
func TestReconcileCountsArchivedChildren(t *testing.T) {
	db, _ := testutil.TempDB(t)

	planID, _, itemID := buildOneItemPlan(t, db, "archive-to-done")

	// Complete the item so the sweep will archive it.
	if err := planning.SetItemStatus(db, itemID, "done", "finished", "test"); err != nil {
		t.Fatal(err)
	}

	// Archive it away. SessionHours 0 with an explicit plan_items table keeps
	// the fixture tight; --yes is required for a real sweep.
	res, err := archive.Run(db, archive.RunOptions{
		SessionHours: 0, Table: "plan_items", Yes: true,
	})
	if err != nil {
		t.Fatalf("archive run: %v", err)
	}
	if res.ByTable["plan_items"] == 0 {
		t.Fatalf("fixture archived nothing: %+v", res)
	}

	var liveItems int
	if err := db.QueryRow(`SELECT COUNT(*) FROM plan_items WHERE plan_id=?`, planID).Scan(&liveItems); err != nil {
		t.Fatal(err)
	}
	if liveItems != 0 {
		t.Fatalf("live items = %d, want 0 after archiving (the repro requires it)", liveItems)
	}

	// Detection must now see the drift rather than "no drift".
	drift, err := planning.FindPlanTreeDrift(db, "archive-to-done")
	if err != nil {
		t.Fatalf("FindPlanTreeDrift: %v", err)
	}
	if len(drift.Milestones) == 0 {
		t.Fatal("no milestone drift found; archived children were not counted")
	}
	foundDone := false
	for _, d := range drift.Milestones {
		if d.ExpectedStatus == "done" {
			foundDone = true
		}
	}
	if !foundDone {
		t.Fatalf("expected a milestone drift to done, got %+v", drift.Milestones)
	}

	// Repair must actually set it.
	if _, err := planning.ReconcilePlans(db, "archive-to-done", true); err != nil {
		t.Fatalf("ReconcilePlans apply: %v", err)
	}

	var planStatus string
	if err := db.QueryRow(`SELECT status FROM plans WHERE id=?`, planID).Scan(&planStatus); err != nil {
		t.Fatal(err)
	}
	if planStatus != "done" {
		t.Fatalf("plan status = %q, want done after reconcile", planStatus)
	}
}

// TestReconcileFullyLivePlanReportsNoDrift is acceptance 3: the archive merge
// must not invent drift for a plan whose children are all still live.
func TestReconcileFullyLivePlanReportsNoDrift(t *testing.T) {
	db, _ := testutil.TempDB(t)

	planID, _, itemID := buildOneItemPlan(t, db, "all-live")
	if err := planning.SetItemStatus(db, itemID, "done", "finished", "test"); err != nil {
		t.Fatal(err)
	}

	// Bring the tree into agreement without archiving anything.
	if _, err := planning.ReconcilePlans(db, "all-live", true); err != nil {
		t.Fatal(err)
	}

	drift, err := planning.FindPlanTreeDrift(db, "all-live")
	if err != nil {
		t.Fatal(err)
	}
	if len(drift.Milestones) != 0 || len(drift.Plans) != 0 {
		t.Fatalf("fully live plan reports drift: %+v", drift)
	}

	var planStatus string
	if err := db.QueryRow(`SELECT status FROM plans WHERE id=?`, planID).Scan(&planStatus); err != nil {
		t.Fatal(err)
	}
	if planStatus != "done" {
		t.Fatalf("plan status = %q, want done", planStatus)
	}
}

// TestReconcilePartiallyArchivedMilestone keeps the mixed case honest: one item
// archived, one still live and in progress. The live in_progress child must win
// over the archived done one.
func TestReconcilePartiallyArchivedMilestone(t *testing.T) {
	db, _ := testutil.TempDB(t)

	planID, err := planning.CreatePlan(db, planning.CreatePlanInput{
		Slug: "partial", Title: "partially archived", Body: "", ModelID: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	msID, err := planning.AddMilestone(db, planID, "M1", "", "test", 0)
	if err != nil {
		t.Fatal(err)
	}
	doneItem, err := planning.AddItem(db, planning.AddItemInput{
		PlanID: planID, MilestoneID: msID, Title: "finished item", Body: "", ModelID: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := planning.SetItemStatus(db, doneItem, "done", "finished", "test"); err != nil {
		t.Fatal(err)
	}
	// Archive only the finished item.
	step := archive.RunOptions{SessionHours: 0, Table: "plan_items", Yes: true}
	if _, err := archive.Run(db, step); err != nil {
		t.Fatal(err)
	}

	// A second item, still in progress, keeps the milestone active.
	liveItem, err := planning.AddItem(db, planning.AddItemInput{
		PlanID: planID, MilestoneID: msID, Title: "ongoing item", Body: "", ModelID: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := planning.StartItem(db, liveItem, "test"); err != nil {
		t.Fatal(err)
	}

	drift, err := planning.FindPlanTreeDrift(db, "partial")
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range drift.Milestones {
		if d.ExpectedStatus == "done" {
			t.Fatalf("milestone expected done while a live child is in progress: %+v", d)
		}
	}

	var planStatus string
	if err := db.QueryRow(`SELECT status FROM plans WHERE id=?`, planID).Scan(&planStatus); err != nil {
		t.Fatal(err)
	}
	if planStatus == "done" {
		t.Fatal("plan marked done while a live child is in progress")
	}
}

// TestReconcileEmptyMilestoneStaysUnknown guards against inventing done: a
// milestone with no children anywhere must derive no status at all.
func TestReconcileEmptyMilestoneStaysUnknown(t *testing.T) {
	db, _ := testutil.TempDB(t)

	planID, err := planning.CreatePlan(db, planning.CreatePlanInput{
		Slug: "empty-ms", Title: "empty milestone", Body: "", ModelID: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := planning.AddMilestone(db, planID, "M1", "", "test", 0); err != nil {
		t.Fatal(err)
	}

	drift, err := planning.FindPlanTreeDrift(db, "empty-ms")
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range drift.Milestones {
		if d.ExpectedStatus == "done" {
			t.Fatalf("empty milestone derived done: %+v", d)
		}
	}
}
