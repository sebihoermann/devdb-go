package planning_test

import (
	"testing"

	"github.com/sebihoermann/devdb-go/internal/domain/planning"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestCreatePlanAutoSlugAndList(t *testing.T) {
	db, _ := testutil.TempDB(t)
	planID, err := planning.CreatePlan(db, planning.CreatePlanInput{Title: "My Cool Plan", ModelID: "test"})
	if err != nil || planID == "" {
		t.Fatalf("create: %s err=%v", planID, err)
	}
	plans, err := planning.ListPlans(db)
	if err != nil || len(plans) != 1 || plans[0].Slug == "" {
		t.Fatalf("plans=%+v err=%v", plans, err)
	}
}

func TestAddMilestoneAndItemErrors(t *testing.T) {
	db, _ := testutil.TempDB(t)
	if _, err := planning.AddMilestone(db, "missing", "M1", "", "test", 1); err == nil {
		t.Fatal("expected missing plan error")
	}
	planID, _ := planning.CreatePlan(db, planning.CreatePlanInput{Slug: "e", Title: "E", ModelID: "test"})
	if _, err := planning.AddItem(db, planning.AddItemInput{PlanID: "missing", Title: "x", ModelID: "test"}); err == nil {
		t.Fatal("expected missing plan for item")
	}
	msID, _ := planning.AddMilestone(db, planID, "M1", "", "test", 1)
	if _, err := planning.AddAcceptance(db, "missing", "x", "test", 1); err == nil {
		t.Fatal("expected missing item for acceptance")
	}
	itemID, _ := planning.AddItem(db, planning.AddItemInput{PlanID: planID, MilestoneID: msID, Title: "t", ModelID: "test"})
	if _, err := planning.SetItemStatusExplicit(db, "missing", "planned", "", "test"); err == nil {
		t.Fatal("expected missing item for status")
	}
	if _, _, err := planning.ShowItem(db, "missing"); err == nil {
		t.Fatal("expected show missing item error")
	}
	if _, _, err := planning.ShowPlan(db, "missing"); err == nil {
		t.Fatal("expected show missing plan error")
	}
	_ = itemID
}

func TestPlanTreeMultipleMilestones(t *testing.T) {
	db, _ := testutil.TempDB(t)
	planID, _ := planning.CreatePlan(db, planning.CreatePlanInput{Slug: "tree", Title: "Tree", ModelID: "test"})
	ms1, _ := planning.AddMilestone(db, planID, "One", "", "test", 1)
	ms2, _ := planning.AddMilestone(db, planID, "Two", "", "test", 2)
	_, _ = planning.AddStructuredItem(db, planning.StructuredItemInput{PlanID: planID, MilestoneID: ms1, Title: "A", ModelID: "test"})
	_, _ = planning.AddStructuredItem(db, planning.StructuredItemInput{PlanID: planID, MilestoneID: ms2, Title: "B", ModelID: "test"})
	tree, err := planning.PlanTree(db, "tree")
	if err != nil || len(tree) != 1 || len(tree[0].Children) != 2 {
		t.Fatalf("tree=%v err=%v", tree, err)
	}
}

func TestReconcileMultipleMilestoneDriftApply(t *testing.T) {
	db, _ := testutil.TempDB(t)
	planID, _ := planning.CreatePlan(db, planning.CreatePlanInput{Slug: "multi", Title: "Multi", ModelID: "test"})
	ms1, _ := planning.AddMilestone(db, planID, "M1", "", "test", 1)
	ms2, _ := planning.AddMilestone(db, planID, "M2", "", "test", 2)
	item1, _ := planning.AddStructuredItem(db, planning.StructuredItemInput{PlanID: planID, MilestoneID: ms1, Title: "A", ModelID: "test"})
	item2, _ := planning.AddStructuredItem(db, planning.StructuredItemInput{PlanID: planID, MilestoneID: ms2, Title: "B", ModelID: "test"})
	_, _ = planning.StartItem(db, item1, "test")
	_, _ = planning.StartItem(db, item2, "test")
	_, _ = db.Exec(`UPDATE milestones SET status='planned' WHERE plan_id=?`, planID)
	result, err := planning.ReconcilePlans(db, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Applied == nil || len(result.Applied.Milestones) < 2 {
		t.Fatalf("applied=%+v", result.Applied)
	}
}

func TestCreatePlanDuplicateSlug(t *testing.T) {
	db, _ := testutil.TempDB(t)
	_, err := planning.CreatePlan(db, planning.CreatePlanInput{Slug: "dup", Title: "One", ModelID: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := planning.CreatePlan(db, planning.CreatePlanInput{Slug: "dup", Title: "Two", ModelID: "test"}); err == nil {
		t.Fatal("expected duplicate slug error")
	}
}

func TestPauseSetStatusAndPlanTreeMissing(t *testing.T) {
	db, _ := testutil.TempDB(t)
	planID, _ := planning.CreatePlan(db, planning.CreatePlanInput{Slug: "life", Title: "Life", ModelID: "test"})
	itemID, _ := planning.AddItem(db, planning.AddItemInput{PlanID: planID, Title: "work", ModelID: "test"})
	_, _ = planning.StartItem(db, itemID, "test")
	if _, err := planning.PauseItem(db, itemID, "save context", "test"); err != nil {
		t.Fatal(err)
	}
	if err := planning.SetItemStatus(db, itemID, "planned", "reset", "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := planning.PlanTree(db, "missing"); err == nil {
		t.Fatal("expected missing plan tree error")
	}
}

func TestScaffoldPlanInvalidMode(t *testing.T) {
	db, _ := testutil.TempDB(t)
	if _, err := planning.ScaffoldPlan(db, planning.ScaffoldPlanInput{
		Title: "Bad", Slug: "bad", Mode: "invalid", RepoRoot: t.TempDir(), ModelID: "test",
	}); err == nil {
		t.Fatal("expected invalid scaffold mode error")
	}
}

func TestFindPlanTreeDriftAllPlans(t *testing.T) {
	db, _ := testutil.TempDB(t)
	planID, _ := planning.CreatePlan(db, planning.CreatePlanInput{Slug: "all", Title: "All", ModelID: "test"})
	msID, _ := planning.AddMilestone(db, planID, "M1", "", "test", 1)
	itemID, _ := planning.AddStructuredItem(db, planning.StructuredItemInput{
		PlanID: planID, MilestoneID: msID, Title: "Task", ModelID: "test",
	})
	_, _ = planning.StartItem(db, itemID, "test")
	_, _ = db.Exec(`UPDATE milestones SET status='planned' WHERE id=?`, msID)
	drift, err := planning.FindPlanTreeDrift(db, "")
	if err != nil {
		t.Fatal(err)
	}
	if drift.IsEmpty() {
		t.Fatal("expected drift across all plans")
	}
}

func TestListItemsMilestoneFilterAndPromoteMissing(t *testing.T) {
	db, _ := testutil.TempDB(t)
	planID, _ := planning.CreatePlan(db, planning.CreatePlanInput{Slug: "f", Title: "F", ModelID: "test"})
	msID, _ := planning.AddMilestone(db, planID, "M1", "", "test", 1)
	itemID, _ := planning.AddItem(db, planning.AddItemInput{PlanID: planID, MilestoneID: msID, Title: "t", ModelID: "test"})
	items, err := planning.ListItems(db, planning.ItemFilter{MilestoneID: msID})
	if err != nil || len(items) != 1 || items[0].ID != itemID {
		t.Fatalf("items=%+v err=%v", items, err)
	}
	if _, err := planning.PromotePlan(db, "missing", "test"); err == nil {
		t.Fatal("expected promote missing plan error")
	}
}
