package planning

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/storage"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestSyncPlanForMilestoneAndPlanDrift(t *testing.T) {
	db, _ := testutil.TempDB(t)
	planID, _ := CreatePlan(db, CreatePlanInput{Slug: "sync", Title: "Sync", ModelID: "test"})
	msID, _ := AddMilestone(db, planID, "M1", "", "test", 1)
	itemID, _ := AddStructuredItem(db, StructuredItemInput{PlanID: planID, MilestoneID: msID, Title: "W", ModelID: "test"})
	_, _ = StartItem(db, itemID, "test")
	_, _ = db.Exec(`UPDATE milestones SET status='planned' WHERE id=?`, msID)
	_, _ = db.Exec(`UPDATE plans SET status='planned' WHERE id=?`, planID)

	report, err := FindPlanTreeDrift(db, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Plans) == 0 && len(report.Milestones) == 0 {
		t.Fatalf("drift=%+v", report)
	}
	res, err := ReconcilePlans(db, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Applied == nil {
		t.Fatalf("reconcile=%+v", res)
	}
	_, _ = syncPlanForMilestone(db, msID)
	_, _ = syncPlanStatus(db, planID)
}

func TestSetItemStatusAndPauseClosePaths(t *testing.T) {
	db, _ := testutil.TempDB(t)
	planID, _ := CreatePlan(db, CreatePlanInput{Slug: "st", Title: "ST", ModelID: "test"})
	msID, _ := AddMilestone(db, planID, "M1", "", "test", 1)
	itemID, _ := AddItem(db, AddItemInput{PlanID: planID, MilestoneID: msID, Title: "S", ModelID: "test"})
	if err := SetItemStatus(db, itemID, "wontfix", "", "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := SetItemStatusExplicit(db, itemID, "done", "", "test"); err != nil {
		t.Fatal(err)
	}
	openID, _ := AddItem(db, AddItemInput{PlanID: planID, MilestoneID: msID, Title: "Open", ModelID: "test"})
	_, _ = StartItem(db, openID, "test")
	if _, err := PauseItem(db, openID, "pause note", "test"); err != nil {
		t.Fatal(err)
	}
}

func TestBackfillAcceptanceAndScaffoldAbsolute(t *testing.T) {
	db, _ := testutil.TempDB(t)
	repo := t.TempDir()
	spec := filepath.Join(repo, "spec.md")
	body := "### M1 Alpha \n\n- [ ] acceptance one\n- [ ] acceptance two\n"
	if err := os.WriteFile(spec, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	planID, _ := CreatePlan(db, CreatePlanInput{Slug: "bf", Title: "BF", ModelID: "test"})
	msID, _ := AddMilestone(db, planID, "M1 Alpha", "", "test", 1)
	_, _ = AddStructuredItem(db, StructuredItemInput{PlanID: planID, MilestoneID: msID, Title: "Work", ModelID: "test"})
	n, err := BackfillAcceptanceFromSpec(db, "M1 Alpha", spec, "test")
	if err != nil || n < 1 {
		t.Fatalf("backfill=%d err=%v", n, err)
	}
	outDir := filepath.Join(repo, "plans")
	_ = os.MkdirAll(outDir, 0o755)
	outPath := filepath.Join(outDir, "abs-plan.html")
	res, err := ScaffoldPlan(db, ScaffoldPlanInput{
		Title: "Abs", Slug: "abs-plan", Mode: "design", MilestoneCount: 1,
		RepoRoot: repo, OutputPath: outPath, ModelID: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Artifact != outPath && !strings.Contains(res.Artifact, "abs-plan.html") {
		t.Fatalf("artifact=%q want %q", res.Artifact, outPath)
	}
}

func TestPromotePlanAndResolveItemID(t *testing.T) {
	db, _ := testutil.TempDB(t)
	repo := t.TempDir()
	skip, err := ScaffoldPlan(db, ScaffoldPlanInput{
		Title: "Promote", Slug: "promote-me", Mode: "implement",
		MilestoneCount: 2, RepoRoot: repo, ModelID: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	promoted, err := PromotePlan(db, skip.Slug, "test")
	if err != nil || promoted.PlanID == "" {
		t.Fatalf("promote=%+v err=%v", promoted, err)
	}
	var firstItem string
	if err := db.QueryRow(`SELECT id FROM plan_items WHERE plan_id=? LIMIT 1`, promoted.PlanID).Scan(&firstItem); err != nil {
		t.Fatal(err)
	}
	id, err := resolveItemID(db, firstItem[:8])
	if err != nil || id == "" {
		t.Fatalf("resolve=%q err=%v", id, err)
	}
}

func TestListPlansMilestonesWithData(t *testing.T) {
	db, _ := testutil.TempDB(t)
	planID, _ := CreatePlan(db, CreatePlanInput{Slug: "lst", Title: "List", ModelID: "test"})
	msID, _ := AddMilestone(db, planID, "M1", "", "test", 1)
	itemID, _ := AddItem(db, AddItemInput{PlanID: planID, MilestoneID: msID, Title: "I", ModelID: "test"})
	_ = storage.NowUTC()
	files, err := ListPlanFiles(db, itemID)
	if err != nil {
		t.Fatal(err)
	}
	_ = files
	plans, err := ListPlans(db)
	if err != nil || len(plans) == 0 {
		t.Fatalf("plans=%d err=%v", len(plans), err)
	}
	ms, err := ListMilestones(db, planID)
	if err != nil || len(ms) == 0 {
		t.Fatalf("milestones=%d err=%v", len(ms), err)
	}
}
