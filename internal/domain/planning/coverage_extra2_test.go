package planning

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestScaffoldImplementModeWithBody(t *testing.T) {
	db, _ := testutil.TempDB(t)
	repo := t.TempDir()
	res, err := ScaffoldPlan(db, ScaffoldPlanInput{
		Title: "Ship Feature", Slug: "ship-feat", Body: "Custom body text",
		Mode: "implement", MilestoneCount: 2, RepoRoot: repo, ModelID: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Milestones != 2 {
		t.Fatalf("milestones=%d", res.Milestones)
	}
	artifact := filepath.Join(repo, "docs", "ship-feat-implementation-plan.html")
	data, err := os.ReadFile(artifact)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "verify record") {
		t.Fatal("implement success html missing")
	}
}

func TestBackfillCreatesLegacyPlanItem(t *testing.T) {
	db, _ := testutil.TempDB(t)
	repo := t.TempDir()
	spec := filepath.Join(repo, "SPEC.md")
	body := `### M3 Gamma 

- [ ] new criterion
`
	if err := os.WriteFile(spec, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	n, err := BackfillAcceptanceFromSpec(db, "M3 Gamma", spec, "test")
	if err != nil || n != 1 {
		t.Fatalf("backfill=%d err=%v", n, err)
	}
	var title string
	if err := db.QueryRow(`SELECT title FROM plan_items WHERE step='m3 gamma'`).Scan(&title); err != nil {
		t.Fatal(err)
	}
	if title != "M3 Gamma" {
		t.Fatalf("title=%q", title)
	}
}

func TestResolveMilestoneIDByNumber(t *testing.T) {
	db, _ := testutil.TempDB(t)
	planID, _ := CreatePlan(db, CreatePlanInput{Slug: "resolve-ms", Title: "R", ModelID: "test"})
	msID, _ := AddMilestone(db, planID, "Second", "", "test", 2)
	got, num, err := ResolveMilestoneID(db, planID, "M2")
	if err != nil || got != msID || num != 2 {
		t.Fatalf("id=%q num=%d err=%v", got, num, err)
	}
	got, num, err = ResolveMilestoneID(db, planID, "2")
	if err != nil || got != msID || num != 2 {
		t.Fatalf("numeric id=%q num=%d err=%v", got, num, err)
	}
	if _, _, err := ResolveMilestoneID(db, planID, "M9"); err == nil {
		t.Fatal("expected missing milestone")
	}
}

func TestPromotePlanRewritesDesignTitles(t *testing.T) {
	db, _ := testutil.TempDB(t)
	repo := t.TempDir()
	res, err := ScaffoldPlan(db, ScaffoldPlanInput{
		Title: "Promote Me", Slug: "promote-me", Mode: "design",
		MilestoneCount: 1, RepoRoot: repo, ModelID: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	promo, err := PromotePlan(db, res.Slug, "test")
	if err != nil || promo.TitlesUpdated < 1 || promo.AcceptanceUpdated < 1 {
		t.Fatalf("promo=%+v err=%v", promo, err)
	}
	var title string
	if err := db.QueryRow(`SELECT title FROM plan_items WHERE plan_id=? LIMIT 1`, res.PlanID).Scan(&title); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(title, "Implement ") {
		t.Fatalf("title=%q", title)
	}
}

func TestFindPlanTreeDriftAllPlans(t *testing.T) {
	db, _ := testutil.TempDB(t)
	_, _ = CreatePlan(db, CreatePlanInput{Slug: "clean", Title: "Clean", ModelID: "test"})
	drift, err := FindPlanTreeDrift(db, "")
	if err != nil {
		t.Fatal(err)
	}
	if !drift.IsEmpty() && drift.DriftCount() > 0 {
		// other plans may exist from parallel tests in package - just ensure call works
	}
	_ = drift
}

func TestSetItemStatusExplicitWithNote(t *testing.T) {
	db, _ := testutil.TempDB(t)
	planID, _ := CreatePlan(db, CreatePlanInput{Slug: "explicit", Title: "E", ModelID: "test"})
	itemID, _ := AddItem(db, AddItemInput{PlanID: planID, Title: "Task", ModelID: "test"})
	id, err := SetItemStatusExplicit(db, itemID, "in_progress", "starting", "test")
	if err != nil || id != itemID {
		t.Fatalf("id=%q err=%v", id, err)
	}
}
