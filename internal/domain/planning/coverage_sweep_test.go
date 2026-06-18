package planning

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestScaffoldAbsoluteOutputAndDesignPromote(t *testing.T) {
	db, _ := testutil.TempDB(t)
	repo := t.TempDir()
	absOut := filepath.Join(repo, "custom", "plan.html")
	res, err := ScaffoldPlan(db, ScaffoldPlanInput{
		Title: "Design Plan", Slug: "design-abs", Mode: "design", MilestoneCount: 1,
		OutputPath: absOut, RepoRoot: repo, ModelID: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(absOut); err != nil {
		t.Fatal(err)
	}
	promo, err := PromotePlan(db, res.Slug, "test")
	if err != nil || promo.TitlesUpdated < 1 {
		t.Fatalf("promote=%+v err=%v", promo, err)
	}
}

func TestBackfillMultiMilestoneSpec(t *testing.T) {
	db, _ := testutil.TempDB(t)
	repo := t.TempDir()
	spec := filepath.Join(repo, "SPEC.md")
	body := `### M1 Alpha 

- [ ] first
- [x] done already

### M2 Beta 

- [ ] second
`
	if err := os.WriteFile(spec, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	n, err := BackfillAcceptanceFromSpec(db, "M1 Alpha", spec, "test")
	if err != nil || n != 2 {
		t.Fatalf("backfill=%d err=%v", n, err)
	}
	n2, err := BackfillAcceptanceFromSpec(db, "M1 Alpha", spec, "test")
	if err != nil || n2 != 0 {
		t.Fatalf("idempotent=%d err=%v", n2, err)
	}
	n3, err := BackfillAcceptanceFromSpec(db, "M9 Missing", spec, "test")
	if err != nil || n3 != 0 {
		t.Fatalf("missing section=%d err=%v", n3, err)
	}
}

func TestPlanFilesRolesAndLegacyList(t *testing.T) {
	db, _ := testutil.TempDB(t)
	planID, _ := CreatePlan(db, CreatePlanInput{Slug: "files", Title: "Files", ModelID: "test"})
	itemID, _ := AddItem(db, AddItemInput{PlanID: planID, Title: "I", ModelID: "test"})
	for _, role := range []string{"create", "modify", "forbidden", "touched"} {
		if _, err := AddPlanFile(db, itemID, role+".go", role, "test"); err != nil {
			t.Fatalf("role %s: %v", role, err)
		}
	}
	if _, err := AddPlanFile(db, itemID, "bad.go", "invalid", "test"); err == nil {
		t.Fatal("bad role")
	}
	files, err := ListPlanFiles(db, itemID)
	if err != nil || len(files) != 4 {
		t.Fatalf("files=%d err=%v", len(files), err)
	}
	_, _ = AddLegacyItem(db, "P", "1", "Legacy", "body", "test")
	legacy, err := ListItems(db, ItemFilter{LegacyOnly: true})
	if err != nil || len(legacy) != 1 {
		t.Fatalf("legacy=%d err=%v", len(legacy), err)
	}
	open, err := ListItems(db, ItemFilter{PlanID: planID, Status: "planned"})
	if err != nil || len(open) != 1 {
		t.Fatalf("filtered=%d err=%v", len(open), err)
	}
}

func TestCloseItemWithEvidenceAndMeetAcceptance(t *testing.T) {
	db, _ := testutil.TempDB(t)
	planID, _ := CreatePlan(db, CreatePlanInput{Slug: "close", Title: "Close", ModelID: "test"})
	itemID, _ := AddItem(db, AddItemInput{PlanID: planID, Title: "Ship", ModelID: "test"})
	accID, _ := AddAcceptance(db, itemID, "works", "test", 0)
	_, _ = MeetAcceptance(db, accID, "tests pass", "test")
	closed, err := CloseItem(db, itemID, "shipped", "test")
	if err != nil || closed != itemID {
		t.Fatalf("close=%q err=%v", closed, err)
	}
}

func TestSyncPlanStatusDirect(t *testing.T) {
	db, _ := testutil.TempDB(t)
	planID, _ := CreatePlan(db, CreatePlanInput{Slug: "syncp", Title: "Sync", ModelID: "test"})
	if status, err := syncPlanStatus(db, planID); err != nil || status != "" {
		t.Fatalf("empty plan status=%q err=%v", status, err)
	}
	msID, _ := AddMilestone(db, planID, "M1", "", "test", 1)
	itemID, _ := AddStructuredItem(db, StructuredItemInput{PlanID: planID, MilestoneID: msID, Title: "W", ModelID: "test"})
	_, _ = StartItem(db, itemID, "test")
	_, _ = db.Exec(`UPDATE milestones SET status='planned' WHERE id=?`, msID)
	if status, err := syncPlanStatus(db, planID); err != nil || status != "active" {
		t.Fatalf("active plan status=%q err=%v", status, err)
	}
}

func TestResolvePlanIDPrefix(t *testing.T) {
	db, _ := testutil.TempDB(t)
	id, _ := CreatePlan(db, CreatePlanInput{Slug: "resolve-me", Title: "R", ModelID: "test"})
	got, err := ResolvePlanID(db, id[:8])
	if err != nil || got != id {
		t.Fatalf("resolve=%q err=%v", got, err)
	}
	got, err = ResolvePlanID(db, "resolve-me")
	if err != nil || got != id {
		t.Fatalf("slug=%q err=%v", got, err)
	}
}

func TestScaffoldSuccessHTMLHelpers(t *testing.T) {
	if !strings.Contains(scaffoldSuccessImplement(), "verify record") {
		t.Fatal("implement html")
	}
	if !strings.Contains(scaffoldSuccessDesign(), "promote") {
		t.Fatal("design html")
	}
}
