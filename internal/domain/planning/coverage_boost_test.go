package planning

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestPauseItemSuccessAndReconcileDryRun(t *testing.T) {
	db, _ := testutil.TempDB(t)
	planID, _ := CreatePlan(db, CreatePlanInput{Slug: "pause", Title: "Pause", ModelID: "test"})
	msID, _ := AddMilestone(db, planID, "M1", "", "test", 1)
	itemID, _ := AddStructuredItem(db, StructuredItemInput{
		PlanID: planID, MilestoneID: msID, Title: "Work", ModelID: "test",
	})
	_, _ = StartItem(db, itemID, "test")
	paused, err := PauseItem(db, itemID, "save context", "test")
	if err != nil || paused != itemID {
		t.Fatalf("pause=%q err=%v", paused, err)
	}
	_, _ = db.Exec(`UPDATE milestones SET status='planned' WHERE id=?`, msID)
	_, _ = db.Exec(`UPDATE plans SET status='planned' WHERE id=?`, planID)
	res, err := ReconcilePlans(db, "pause", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Drift.Milestones) == 0 && len(res.Drift.Plans) == 0 {
		t.Fatalf("expected drift: %+v", res)
	}
	if res.Applied != nil {
		t.Fatal("dry-run should not apply")
	}
}

func TestPromotePlanDesignPrefix(t *testing.T) {
	db, _ := testutil.TempDB(t)
	repo := t.TempDir()
	res, err := ScaffoldPlan(db, ScaffoldPlanInput{
		Title: "Design work", Slug: "design-promote", Mode: "design",
		MilestoneCount: 1, RepoRoot: repo, ModelID: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	promo, err := PromotePlan(db, res.Slug, "test")
	if err != nil || promo.TitlesUpdated < 1 {
		t.Fatalf("promote=%+v err=%v", promo, err)
	}
	var title string
	if err := db.QueryRow(`SELECT title FROM plan_items WHERE plan_id=? LIMIT 1`, promo.PlanID).Scan(&title); err != nil {
		t.Fatal(err)
	}
	if title == "" || title[:9] != "Implement" {
		t.Fatalf("title=%q", title)
	}
}

func TestSyncMilestoneAndPlanHelpers(t *testing.T) {
	db, _ := testutil.TempDB(t)
	planID, _ := CreatePlan(db, CreatePlanInput{Slug: "sync-h", Title: "Sync", ModelID: "test"})
	msID, _ := AddMilestone(db, planID, "M1", "", "test", 1)
	itemID, _ := AddStructuredItem(db, StructuredItemInput{
		PlanID: planID, MilestoneID: msID, Title: "W", ModelID: "test",
	})
	_, _ = StartItem(db, itemID, "test")
	_, _ = db.Exec(`UPDATE milestones SET status='planned' WHERE id=?`, msID)
	if status, err := syncMilestoneStatus(db, msID); err != nil || status != "in_progress" {
		t.Fatalf("milestone sync=%q err=%v", status, err)
	}
	_, _ = db.Exec(`UPDATE plans SET status='planned' WHERE id=?`, planID)
	if status, err := syncPlanForMilestone(db, msID); err != nil || status != "active" {
		t.Fatalf("plan for milestone=%q err=%v", status, err)
	}
	if status, err := syncPlanStatus(db, planID); err != nil || status != "active" {
		t.Fatalf("plan sync=%q err=%v", status, err)
	}
}

func TestResolveMilestoneIDBranches(t *testing.T) {
	db, _ := testutil.TempDB(t)
	planID, _ := CreatePlan(db, CreatePlanInput{Slug: "rms", Title: "R", ModelID: "test"})
	msID, _ := AddMilestone(db, planID, "M1 Alpha", "", "test", 1)
	got, num, err := ResolveMilestoneID(db, planID, "M1")
	if err != nil || got != msID || num != 1 {
		t.Fatalf("prefix=%q num=%d err=%v", got, num, err)
	}
	got, _, err = ResolveMilestoneID(db, planID, msID[:8])
	if err != nil || got != msID {
		t.Fatalf("id prefix=%q err=%v", got, err)
	}
	_, _, err = ResolveMilestoneID(db, planID, "M9")
	if err == nil {
		t.Fatal("expected missing milestone error")
	}
}

func TestAddStructuredItemAndLegacyPaths(t *testing.T) {
	db, _ := testutil.TempDB(t)
	planID, _ := CreatePlan(db, CreatePlanInput{Slug: "struct", Title: "S", ModelID: "test"})
	msID, _ := AddMilestone(db, planID, "M1", "", "test", 1)
	id, err := AddStructuredItem(db, StructuredItemInput{
		PlanID: planID, MilestoneID: msID, Title: "Structured", Body: "body", ModelID: "test",
	})
	if err != nil || id == "" {
		t.Fatalf("structured=%q err=%v", id, err)
	}
	legacy, err := AddLegacyItem(db, "P", "2", "Legacy two", "notes", "test")
	if err != nil || legacy == "" {
		t.Fatalf("legacy=%q err=%v", legacy, err)
	}
}

func TestBackfillMissingSpecFile(t *testing.T) {
	db, _ := testutil.TempDB(t)
	planID, _ := CreatePlan(db, CreatePlanInput{Slug: "bf-miss", Title: "BF", ModelID: "test"})
	_, _ = AddMilestone(db, planID, "M1", "", "test", 1)
	n, err := BackfillAcceptanceFromSpec(db, "M1", filepath.Join(t.TempDir(), "missing.md"), "test")
	if err == nil {
		t.Fatalf("expected missing spec error, n=%d", n)
	}
}

func TestScaffoldImplementAbsolutePath(t *testing.T) {
	db, _ := testutil.TempDB(t)
	repo := t.TempDir()
	out := filepath.Join(repo, "plans", "impl.html")
	res, err := ScaffoldPlan(db, ScaffoldPlanInput{
		Title: "Impl abs", Slug: "impl-abs", Mode: "implement",
		MilestoneCount: 2, OutputPath: out, RepoRoot: repo, ModelID: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Artifact == "" {
		t.Fatal("empty artifact path")
	}
	if _, err := os.Stat(filepath.Join(repo, res.Artifact)); err != nil {
		t.Fatalf("artifact: %v", err)
	}
}

func TestSetItemStatusAndShowBranches(t *testing.T) {
	db, _ := testutil.TempDB(t)
	planID, _ := CreatePlan(db, CreatePlanInput{Slug: "show", Title: "Show", ModelID: "test"})
	itemID, _ := AddItem(db, AddItemInput{PlanID: planID, Title: "Item", ModelID: "test"})
	if err := SetItemStatus(db, itemID, "planned", "", "test"); err != nil {
		t.Fatal(err)
	}
	item, acc, err := ShowItem(db, itemID[:8])
	if err != nil || item.ID != itemID {
		t.Fatalf("show=%+v acc=%d err=%v", item, len(acc), err)
	}
	accID, _ := AddAcceptance(db, itemID, "works", "test", 1)
	item, acc, err = ShowItem(db, itemID)
	if err != nil || len(acc) != 1 || acc[0].ID != accID {
		t.Fatalf("show acc=%+v err=%v", acc, err)
	}
}
