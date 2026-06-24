package planning_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/domain/planning"
	"github.com/sebihoermann/devdb-go/internal/migrate"
	"github.com/sebihoermann/devdb-go/internal/storage"
)

func TestScaffoldDesignAndPromote(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := storage.Open(filepath.Join(dir, "development.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := migrate.RunAll(db); err != nil {
		t.Fatal(err)
	}

	result, err := planning.ScaffoldPlan(db, planning.ScaffoldPlanInput{
		Title: "Test Feature", Slug: "test-feature", Mode: "design",
		MilestoneCount: 2, RepoRoot: repo, ModelID: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.PlanID == "" || result.Milestones != 2 {
		t.Fatalf("unexpected scaffold result: %+v", result)
	}
	artifact := filepath.Join(repo, result.Artifact)
	if _, err := os.Stat(artifact); err != nil {
		t.Fatalf("artifact missing: %v", err)
	}

	promote, err := planning.PromotePlan(db, result.Slug, "test")
	if err != nil {
		t.Fatal(err)
	}
	if promote.TitlesUpdated != 2 {
		t.Fatalf("titles_updated=%d want 2", promote.TitlesUpdated)
	}
	if promote.AcceptanceUpdated != 2 {
		t.Fatalf("acceptance_updated=%d want 2", promote.AcceptanceUpdated)
	}

	items, err := planning.ListItems(db, planning.ItemFilter{PlanID: promote.PlanID, Limit: 10})
	if err != nil || len(items) == 0 {
		t.Fatal("no items")
	}
	item, acc, err := planning.ShowItem(db, items[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	_ = item
	if len(acc) == 0 || acc[0].Criterion != planning.ScaffoldAcceptanceImplement {
		t.Fatalf("acceptance not promoted: %+v", acc)
	}
	if !strings.HasPrefix(item.Title, "Implement ") {
		t.Fatalf("title not promoted: %q", item.Title)
	}
}

func TestReconcilePlanTreeDrift(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "development.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := migrate.RunAll(db); err != nil {
		t.Fatal(err)
	}

	planID, err := planning.CreatePlan(db, planning.CreatePlanInput{
		Slug: "drift", Title: "Drift plan", ModelID: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	msID, err := planning.AddMilestone(db, planID, "M1", "", "test", 1)
	if err != nil {
		t.Fatal(err)
	}
	itemID, err := planning.AddStructuredItem(db, planning.StructuredItemInput{
		PlanID: planID, MilestoneID: msID, Title: "Ship it", ModelID: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	accID, err := planning.AddAcceptance(db, itemID, "shipped", "test", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := planning.MeetAcceptance(db, accID, "ok", "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := planning.CloseItem(db, itemID, "done", "test"); err != nil {
		t.Fatal(err)
	}

	drift, err := planning.FindPlanTreeDrift(db, "")
	if err != nil {
		t.Fatal(err)
	}
	if !drift.IsEmpty() {
		t.Fatalf("auto-cascade should leave no drift after close; got %+v", drift)
	}

	// Manually rewind milestone + plan to introduce drift, then verify
	// reconcile still repairs it as a safety net for any path that bypassed
	// CloseItem (legacy items, direct DB writes, etc.).
	if _, err := db.Exec(`UPDATE milestones SET status='planned' WHERE id=?`, msID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE plans SET status='active' WHERE id=?`, planID); err != nil {
		t.Fatal(err)
	}
	drift, err = planning.FindPlanTreeDrift(db, "")
	if err != nil {
		t.Fatal(err)
	}
	if drift.IsEmpty() {
		t.Fatal("expected drift after manually rewinding milestone/plan status")
	}

	result, err := planning.ReconcilePlans(db, "drift", true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Applied == nil {
		t.Fatal("expected applied reconcile")
	}
	after, err := planning.FindPlanTreeDrift(db, "drift")
	if err != nil {
		t.Fatal(err)
	}
	if !after.IsEmpty() {
		t.Fatalf("drift remains after apply: %+v", after)
	}
}

func TestBackfillAcceptanceFromSpec(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "development.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := migrate.RunAll(db); err != nil {
		t.Fatal(err)
	}

	spec := filepath.Join(t.TempDir(), "SPEC.md")
	content := `## Plan

### M1 First milestone

- [ ] criterion one
- [ ] criterion two

### M2 Second
`
	if err := os.WriteFile(spec, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	count, err := planning.BackfillAcceptanceFromSpec(db, "M1", spec, "test")
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("created=%d want 2", count)
	}

	count2, err := planning.BackfillAcceptanceFromSpec(db, "M1", spec, "test")
	if err != nil {
		t.Fatal(err)
	}
	if count2 != 0 {
		t.Fatalf("duplicate backfill created %d rows", count2)
	}
}
