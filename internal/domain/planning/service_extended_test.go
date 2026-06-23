package planning_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/domain/planning"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestResolvePlanAndMilestone(t *testing.T) {
	db, _ := testutil.TempDB(t)
	planID, err := planning.CreatePlan(db, planning.CreatePlanInput{
		Slug: "my-plan", Title: "My Plan", ModelID: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	msID, err := planning.AddMilestone(db, planID, "First", "", "test", 1)
	if err != nil {
		t.Fatal(err)
	}

	resolved, err := planning.ResolvePlanID(db, "my-plan")
	if err != nil || resolved != planID {
		t.Fatalf("resolve plan: %s err=%v", resolved, err)
	}
	resolved, err = planning.ResolvePlanID(db, planID[:8])
	if err != nil || resolved != planID {
		t.Fatalf("resolve by prefix: %s err=%v", resolved, err)
	}

	msResolved, num, err := planning.ResolveMilestoneID(db, planID, "M1")
	if err != nil || msResolved != msID || num != 1 {
		t.Fatalf("resolve M1: id=%s num=%d err=%v", msResolved, num, err)
	}
	msResolved, _, err = planning.ResolveMilestoneID(db, planID, msID[:8])
	if err != nil || msResolved != msID {
		t.Fatalf("resolve ms prefix: %s err=%v", msResolved, err)
	}
}

func TestResolveMilestoneNotFound(t *testing.T) {
	db, _ := testutil.TempDB(t)
	planID, _ := planning.CreatePlan(db, planning.CreatePlanInput{Slug: "p", Title: "P", ModelID: "test"})
	if _, _, err := planning.ResolveMilestoneID(db, planID, "M99"); err == nil {
		t.Fatal("expected milestone not found")
	}
	if _, err := planning.ResolvePlanID(db, "missing"); err == nil {
		t.Fatal("expected plan not found")
	}
}

func TestServiceCRUDAndLifecycle(t *testing.T) {
	db, _ := testutil.TempDB(t)

	planID, err := planning.CreatePlan(db, planning.CreatePlanInput{
		Title: "Auto Slug Plan", Body: "details", ModelID: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	plans, err := planning.ListPlans(db)
	if err != nil || len(plans) != 1 {
		t.Fatalf("list plans: %d err=%v", len(plans), err)
	}

	msID, err := planning.AddMilestone(db, planID, "Milestone 1", "body", "test", 0)
	if err != nil {
		t.Fatal(err)
	}
	milestones, err := planning.ListMilestones(db, planID)
	if err != nil || len(milestones) != 1 {
		t.Fatalf("milestones: %d err=%v", len(milestones), err)
	}

	itemID, err := planning.AddItem(db, planning.AddItemInput{
		PlanID: planID, MilestoneID: msID, Title: "Build it", Body: "work", ModelID: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := planning.AddAcceptance(db, itemID, "works", "test", 0); err != nil {
		t.Fatal(err)
	}

	started, err := planning.StartItem(db, itemID[:8], "test")
	if err != nil || started != itemID {
		t.Fatalf("start: %s err=%v", started, err)
	}
	inFlight, err := planning.InFlight(db)
	if err != nil || inFlight == nil || inFlight.ID != itemID {
		t.Fatalf("in-flight=%+v err=%v", inFlight, err)
	}

	if _, err := planning.PauseItem(db, itemID, "context switch", "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := planning.SetItemStatusExplicit(db, itemID, "planned", "reset", "test"); err != nil {
		t.Fatal(err)
	}

	fileID, err := planning.AddPlanFile(db, itemID, "main.go", "modify", "test")
	if err != nil || fileID == "" {
		t.Fatalf("add plan file: %s err=%v", fileID, err)
	}
	files, err := planning.ListPlanFiles(db, itemID)
	if err != nil || len(files) != 1 {
		t.Fatalf("plan files: %d err=%v", len(files), err)
	}
	if _, err := planning.AddPlanFile(db, itemID, "x.go", "bad-role", "test"); err == nil {
		t.Fatal("expected invalid role error")
	}

	p, ms, err := planning.ShowPlan(db, planID[:8])
	if err != nil || p.ID != planID || len(ms) != 1 {
		t.Fatalf("show plan: p=%+v ms=%d err=%v", p, len(ms), err)
	}
	tree, err := planning.PlanTree(db, plans[0].Slug)
	if err != nil || len(tree) != 1 || len(tree[0].Children) < 1 {
		t.Fatalf("tree=%+v err=%v", tree, err)
	}

	legacyID, err := planning.AddLegacyItem(db, "M2", "1", "legacy task", "", "", "test")
	if err != nil {
		t.Fatal(err)
	}
	legacyItems, err := planning.ListItems(db, planning.ItemFilter{LegacyOnly: true})
	if err != nil || len(legacyItems) != 1 || legacyItems[0].ID != legacyID {
		t.Fatalf("legacy items: %+v err=%v", legacyItems, err)
	}
}

func TestPauseItemRequiresNote(t *testing.T) {
	db, _ := testutil.TempDB(t)
	planID, _ := planning.CreatePlan(db, planning.CreatePlanInput{Slug: "p", Title: "P", ModelID: "test"})
	itemID, _ := planning.AddItem(db, planning.AddItemInput{PlanID: planID, Title: "t", ModelID: "test"})
	_, _ = planning.StartItem(db, itemID, "test")
	if _, err := planning.PauseItem(db, itemID, "", "test"); err == nil {
		t.Fatal("expected note required error")
	}
}

func TestReconcileDriftCount(t *testing.T) {
	db, _ := testutil.TempDB(t)
	planID, _ := planning.CreatePlan(db, planning.CreatePlanInput{Slug: "r", Title: "R", ModelID: "test"})
	msID, _ := planning.AddMilestone(db, planID, "M1", "", "test", 1)
	itemID, _ := planning.AddStructuredItem(db, planning.StructuredItemInput{
		PlanID: planID, MilestoneID: msID, Title: "Task", ModelID: "test",
	})
	accID, _ := planning.AddAcceptance(db, itemID, "done", "test", 1)
	_, _ = planning.MeetAcceptance(db, accID, "ok", "test")
	_, _ = planning.CloseItem(db, itemID, "shipped", "test")

	drift, err := planning.FindPlanTreeDrift(db, "")
	if err != nil {
		t.Fatal(err)
	}
	if drift.DriftCount() < 1 {
		t.Fatalf("drift count=%d", drift.DriftCount())
	}
	result, err := planning.ReconcilePlans(db, "r", false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Applied != nil {
		t.Fatal("dry reconcile should not apply")
	}
}

func TestBackfillSpecNotFound(t *testing.T) {
	db, _ := testutil.TempDB(t)
	if _, err := planning.BackfillAcceptanceFromSpec(db, "M1", "/no/such/spec.md", "test"); err == nil {
		t.Fatal("expected spec not found error")
	}
}

func TestBackfillNoMilestoneSection(t *testing.T) {
	db, _ := testutil.TempDB(t)
	spec := filepath.Join(t.TempDir(), "SPEC.md")
	if err := os.WriteFile(spec, []byte("# empty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	count, err := planning.BackfillAcceptanceFromSpec(db, "M9", spec, "test")
	if err != nil || count != 0 {
		t.Fatalf("count=%d err=%v", count, err)
	}
}

func TestScaffoldImplementMode(t *testing.T) {
	db, _ := testutil.TempDB(t)
	repo := t.TempDir()
	result, err := planning.ScaffoldPlan(db, planning.ScaffoldPlanInput{
		Title: "Feature X", Slug: "feature-x", Mode: "implement",
		MilestoneCount: 1, RepoRoot: repo, ModelID: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.PlanID == "" {
		t.Fatal("expected plan id")
	}
}

func TestScaffoldDesignModeAndPromote(t *testing.T) {
	db, _ := testutil.TempDB(t)
	repo := t.TempDir()
	result, err := planning.ScaffoldPlan(db, planning.ScaffoldPlanInput{
		Title: "Design Y", Slug: "design-y", Mode: "design",
		MilestoneCount: 2, RepoRoot: repo, ModelID: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	tree, err := planning.PlanTree(db, "design-y")
	if err != nil || len(tree) == 0 {
		t.Fatalf("tree=%v err=%v", tree, err)
	}
	promoted, err := planning.PromotePlan(db, result.PlanID, "test")
	if err != nil {
		t.Fatal(err)
	}
	if promoted.PlanID == "" {
		t.Fatal("expected promoted plan id")
	}
}

func TestStartItemAndMeetAcceptanceErrors(t *testing.T) {
	db, _ := testutil.TempDB(t)
	planID, _ := planning.CreatePlan(db, planning.CreatePlanInput{Slug: "e", Title: "E", ModelID: "test"})
	itemID, _ := planning.AddItem(db, planning.AddItemInput{PlanID: planID, Title: "t", ModelID: "test"})
	if _, err := planning.StartItem(db, "missing", "test"); err == nil {
		t.Fatal("expected start missing item error")
	}
	_, _ = planning.StartItem(db, itemID, "test")
	if _, err := planning.MeetAcceptance(db, "missing-acc", "evidence", "test"); err == nil {
		t.Fatal("expected meet acceptance error")
	}
	if _, err := planning.CloseItem(db, "missing", "done", "test"); err == nil {
		t.Fatal("expected close item error")
	}
}
