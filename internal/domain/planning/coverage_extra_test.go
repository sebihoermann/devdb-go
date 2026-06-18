package planning

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestScaffoldInvalidModeAndDuplicateSlug(t *testing.T) {
	db, _ := testutil.TempDB(t)
	repo := t.TempDir()
	if _, err := ScaffoldPlan(db, ScaffoldPlanInput{
		Title: "Bad", Mode: "invalid", RepoRoot: repo, ModelID: "test",
	}); err == nil || !strings.Contains(err.Error(), "mode must be") {
		t.Fatalf("mode err=%v", err)
	}
	_, err := ScaffoldPlan(db, ScaffoldPlanInput{
		Title: "First", Slug: "dup-slug", RepoRoot: repo, ModelID: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = ScaffoldPlan(db, ScaffoldPlanInput{
		Title: "Second", Slug: "dup-slug", RepoRoot: repo, ModelID: "test",
	})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("dup err=%v", err)
	}
}

func TestScaffoldDefaultSlugFromTitle(t *testing.T) {
	db, _ := testutil.TempDB(t)
	repo := t.TempDir()
	res, err := ScaffoldPlan(db, ScaffoldPlanInput{
		Title: "My Long Implementation Plan Title Here", RepoRoot: repo, ModelID: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Slug == "" {
		t.Fatal("empty slug")
	}
}

func TestReconcilePlansDryRunOnly(t *testing.T) {
	db, _ := testutil.TempDB(t)
	planID, _ := CreatePlan(db, CreatePlanInput{Slug: "dry", Title: "Dry", ModelID: "test"})
	msID, _ := AddMilestone(db, planID, "M1", "", "test", 1)
	itemID, _ := AddStructuredItem(db, StructuredItemInput{PlanID: planID, MilestoneID: msID, Title: "W", ModelID: "test"})
	_, _ = StartItem(db, itemID, "test")
	_, _ = db.Exec(`UPDATE milestones SET status='planned' WHERE id=?`, msID)
	result, err := ReconcilePlans(db, "dry", false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Applied != nil || result.Drift.IsEmpty() {
		t.Fatalf("dry run=%+v", result)
	}
}

func TestBackfillAcceptanceReadError(t *testing.T) {
	db, _ := testutil.TempDB(t)
	if _, err := BackfillAcceptanceFromSpec(db, "M1", "/no/such/spec.md", "test"); err == nil {
		t.Fatal("expected read error")
	}
}

func TestAddStructuredItemBadMilestone(t *testing.T) {
	db, _ := testutil.TempDB(t)
	planID, _ := CreatePlan(db, CreatePlanInput{Slug: "struct", Title: "S", ModelID: "test"})
	if _, err := AddStructuredItem(db, StructuredItemInput{
		PlanID: planID, MilestoneID: "missing", Title: "X", ModelID: "test",
	}); err == nil {
		t.Fatal("expected milestone error")
	}
}

func TestScaffoldSkipAcceptance(t *testing.T) {
	db, _ := testutil.TempDB(t)
	repo := t.TempDir()
	res, err := ScaffoldPlan(db, ScaffoldPlanInput{
		Title: "Skip", Slug: "skip-acc", SkipAcceptance: true, RepoRoot: repo, ModelID: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	artifact := filepath.Join(repo, "docs", "skip-acc-implementation-plan.html")
	if _, err := os.Stat(artifact); err != nil {
		t.Fatalf("artifact: %v", err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM plan_item_acceptance WHERE plan_item_id IN (
		SELECT id FROM plan_items WHERE plan_id=?)`, res.PlanID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("acceptance rows=%d", n)
	}
}
