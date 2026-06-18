package planning

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestMilestoneStatusAllBranches(t *testing.T) {
	db, _ := testutil.TempDB(t)
	planID, _ := CreatePlan(db, CreatePlanInput{Slug: "br", Title: "Branches", ModelID: "test"})
	msID, _ := AddMilestone(db, planID, "M1", "", "test", 1)

	// empty milestone with done parent plan
	_, _ = db.Exec(`UPDATE plans SET status='done' WHERE id=?`, planID)
	status, err := milestoneStatusFromChildren(db, msID)
	if err != nil || status != "done" {
		t.Fatalf("empty+done parent: %q err=%v", status, err)
	}
	_, _ = db.Exec(`UPDATE plans SET status='active' WHERE id=?`, planID)

	addItem := func(title, st string) {
		id, _ := AddStructuredItem(db, StructuredItemInput{PlanID: planID, MilestoneID: msID, Title: title, ModelID: "test"})
		_, _ = db.Exec(`UPDATE plan_items SET status=? WHERE id=?`, st, id)
	}
	addItem("wip", "in_progress")
	if s, _ := milestoneStatusFromChildren(db, msID); s != "in_progress" {
		t.Fatalf("in_progress=%q", s)
	}
	_, _ = db.Exec(`UPDATE plan_items SET status='done' WHERE milestone_id=?`, msID)
	addItem("planned", "planned")
	if s, _ := milestoneStatusFromChildren(db, msID); s != "planned" {
		t.Fatalf("planned=%q", s)
	}
	_, _ = db.Exec(`UPDATE plan_items SET status='done' WHERE milestone_id=?`, msID)
	addItem("done", "done")
	if s, _ := milestoneStatusFromChildren(db, msID); s != "done" {
		t.Fatalf("done=%q", s)
	}
	_, _ = db.Exec(`UPDATE plan_items SET status='done' WHERE milestone_id=?`, msID)
	addItem("wontfix", "wontfix")
	if s, _ := milestoneStatusFromChildren(db, msID); s != "done" && s != "wontfix" {
		t.Fatalf("wontfix/done=%q", s)
	}
}

func TestPlanStatusFromMilestonesBranches(t *testing.T) {
	db, _ := testutil.TempDB(t)
	planID, _ := CreatePlan(db, CreatePlanInput{Slug: "ps", Title: "PS", ModelID: "test"})
	ms1, _ := AddMilestone(db, planID, "M1", "", "test", 1)
	ms2, _ := AddMilestone(db, planID, "M2", "", "test", 2)
	_, _ = db.Exec(`UPDATE milestones SET status='in_progress' WHERE id=?`, ms1)
	if s, _ := planStatusFromMilestones(db, planID); s != "active" {
		t.Fatalf("active=%q", s)
	}
	_, _ = db.Exec(`UPDATE milestones SET status='done' WHERE id IN (?,?)`, ms1, ms2)
	if s, _ := planStatusFromMilestones(db, planID); s != "done" {
		t.Fatalf("done=%q", s)
	}
}

func TestFindPlanTreeDriftAndReconcile(t *testing.T) {
	db, _ := testutil.TempDB(t)
	planID, _ := CreatePlan(db, CreatePlanInput{Slug: "drift", Title: "Drift", ModelID: "test"})
	msID, _ := AddMilestone(db, planID, "M1", "", "test", 1)
	itemID, _ := AddStructuredItem(db, StructuredItemInput{PlanID: planID, MilestoneID: msID, Title: "Work", ModelID: "test"})
	_, _ = StartItem(db, itemID, "test")
	_, _ = db.Exec(`UPDATE milestones SET status='planned' WHERE id=?`, msID)
	_, _ = db.Exec(`UPDATE plans SET status='planned' WHERE id=?`, planID)

	report, err := FindPlanTreeDrift(db, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Milestones) == 0 {
		t.Fatalf("drift=%+v", report)
	}
	bySlug, err := FindPlanTreeDrift(db, "drift")
	if err != nil || len(bySlug.Milestones) == 0 {
		t.Fatalf("by slug: %+v err=%v", bySlug, err)
	}
	res, err := ReconcilePlans(db, "drift", true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Applied == nil || len(res.Applied.Milestones) == 0 {
		t.Fatalf("applied=%+v", res)
	}
}

func TestScaffoldPromoteAndServiceEdges(t *testing.T) {
	db, _ := testutil.TempDB(t)
	repo := t.TempDir()
	res, err := ScaffoldPlan(db, ScaffoldPlanInput{
		Title: "Impl", Slug: "impl-plan", Mode: "implement", MilestoneCount: 2,
		RepoRoot: repo, ModelID: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(repo, res.Artifact)); err != nil {
		t.Fatalf("artifact missing: %v", err)
	}
	if _, err := ScaffoldPlan(db, ScaffoldPlanInput{Title: "Bad", Mode: "nope", ModelID: "test"}); err == nil {
		t.Fatal("expected bad mode")
	}

	skip, err := ScaffoldPlan(db, ScaffoldPlanInput{
		Title: "Skip", Slug: "skip-acc", Mode: "design", SkipAcceptance: true,
		MilestoneCount: 1, RepoRoot: repo, ModelID: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	promoted, err := PromotePlan(db, skip.Slug, "test")
	if err != nil || promoted.TitlesUpdated < 0 {
		t.Fatalf("promote: %+v err=%v", promoted, err)
	}

	planID, _ := CreatePlan(db, CreatePlanInput{Title: "Auto Slug", ModelID: "test"})
	msID, _ := AddMilestone(db, planID, "M1", "", "test", 1)
	legacyID, _ := AddLegacyItem(db, "P", "1", "L", "", "test")
	_ = legacyID
	itemID, _ := AddItem(db, AddItemInput{PlanID: planID, MilestoneID: msID, Title: "I", ModelID: "test"})
	accID, _ := AddAcceptance(db, itemID, "criterion", "test", 0)
	_, _ = MeetAcceptance(db, accID, "evidence", "test")

	if _, err := PauseItem(db, itemID, "", "test"); err == nil {
		t.Fatal("pause requires note")
	}
	openItem, _ := AddItem(db, AddItemInput{PlanID: planID, MilestoneID: msID, Title: "Open", ModelID: "test"})
	_, _ = AddAcceptance(db, openItem, "still open", "test", 1)
	if _, err := CloseItem(db, openItem, "", "test"); err == nil {
		t.Fatal("close with open acceptance should fail")
	}
	_, _ = CloseItem(db, itemID, "shipped", "test")

	tree, err := PlanTree(db, planID[:8])
	if err != nil || len(tree) == 0 {
		t.Fatalf("tree=%+v err=%v", tree, err)
	}
	plans, _ := ListPlans(db)
	if len(plans) == 0 {
		t.Fatal("list plans empty")
	}
	ms, _ := ListMilestones(db, planID)
	if len(ms) == 0 {
		t.Fatal("list milestones empty")
	}
	items, _ := ListItems(db, ItemFilter{PlanID: planID})
	if len(items) < 2 {
		t.Fatalf("items=%d", len(items))
	}
	inFlight, _ := InFlight(db)
	if inFlight != nil {
		t.Fatalf("expected no in-flight after close: %+v", inFlight)
	}
	files, _ := ListPlanFiles(db, itemID)
	_ = files
	_, _ = AddPlanFile(db, itemID, "x.go", "modify", "test")
	_, _ = SetItemStatusExplicit(db, itemID, "wontfix", "later", "test")
}

func TestBackfillAcceptanceAndResolveMilestone(t *testing.T) {
	db, _ := testutil.TempDB(t)
	planID, _ := CreatePlan(db, CreatePlanInput{Slug: "bf", Title: "BF", ModelID: "test"})
	_, _ = AddMilestone(db, planID, "M1 Alpha", "", "test", 1)
	repo := t.TempDir()
	spec := filepath.Join(repo, "SPEC.md")
	body := "### M1 Alpha\n\n- [ ] one\n- [ ] two\n"
	if err := os.WriteFile(spec, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	n, err := BackfillAcceptanceFromSpec(db, "M1", spec, "test")
	if err != nil || n != 2 {
		t.Fatalf("backfill=%d err=%v", n, err)
	}
	msID, _, err := ResolveMilestoneID(db, planID, "M1")
	if err != nil || msID == "" {
		t.Fatalf("resolve ms=%q err=%v", msID, err)
	}
	_, _, err = ResolveMilestoneID(db, planID, "missing")
	if err == nil {
		t.Fatal("expected missing milestone error")
	}
}

func TestSlugifyLongTitle(t *testing.T) {
	long := strings.Repeat("word ", 20)
	s := slugify(long)
	if !strings.HasPrefix(s, "word") {
		t.Fatalf("slug=%q", s)
	}
	if nullStr("  ") != nil {
		t.Fatal("whitespace should be nil")
	}
}
