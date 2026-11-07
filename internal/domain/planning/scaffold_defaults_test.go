package planning

import (
	"testing"

	"github.com/sebihoermann/devdb-go/internal/testutil"
)

// TestScaffoldDefaultNoMetaItems pins the post-2026-11 scaffold contract
// (feedback 70d31c45, skill pitfalls 15/19):
//   - default scaffold creates milestones only — no auto
//     "Implement/Design Milestone N" meta-items (unclosable debris that
//     inflated plan counts and blocked plan closure);
//   - milestone titles carry no "M<n>. " prefix (plan show/tree already
//     render the number, so the old shape displayed "M1 M1. Milestone 1");
//   - --with-implement-items (WithImplementItems: true) restores the
//     legacy shape for the promote flow.
func TestScaffoldDefaultNoMetaItems(t *testing.T) {
	db, _ := testutil.TempDB(t)
	repo := t.TempDir()

	res, err := ScaffoldPlan(db, ScaffoldPlanInput{
		Title: "Clean Scaffold", Slug: "clean-scaffold", Mode: "implement",
		MilestoneCount: 3, RepoRoot: repo, ModelID: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Milestones != 3 {
		t.Fatalf("milestones=%d want 3", res.Milestones)
	}

	var items int
	if err := db.QueryRow(`SELECT COUNT(*) FROM plan_items WHERE plan_id=?`, res.PlanID).Scan(&items); err != nil {
		t.Fatal(err)
	}
	if items != 0 {
		t.Fatalf("default scaffold created %d meta-items, want 0", items)
	}

	var badTitles int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM milestones WHERE plan_id=? AND title LIKE 'M%.%'`, res.PlanID).Scan(&badTitles); err != nil {
		t.Fatal(err)
	}
	if badTitles != 0 {
		t.Fatalf("%d milestone titles still carry the M<n>. prefix", badTitles)
	}

	var title string
	if err := db.QueryRow(
		`SELECT title FROM milestones WHERE plan_id=? AND number=1`, res.PlanID).Scan(&title); err != nil {
		t.Fatal(err)
	}
	if title != "Milestone 1" {
		t.Fatalf("milestone 1 title = %q, want %q", title, "Milestone 1")
	}
}

func TestScaffoldWithImplementItemsLegacyShape(t *testing.T) {
	db, _ := testutil.TempDB(t)
	repo := t.TempDir()

	res, err := ScaffoldPlan(db, ScaffoldPlanInput{
		Title: "Legacy Scaffold", Slug: "legacy-scaffold", Mode: "design",
		MilestoneCount: 2, RepoRoot: repo, ModelID: "test", WithImplementItems: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	var items int
	if err := db.QueryRow(`SELECT COUNT(*) FROM plan_items WHERE plan_id=?`, res.PlanID).Scan(&items); err != nil {
		t.Fatal(err)
	}
	if items != 2 {
		t.Fatalf("legacy scaffold created %d items, want 2 (one per milestone)", items)
	}

	var designPrefixed int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM plan_items WHERE plan_id=? AND title LIKE 'Design %'`, res.PlanID).Scan(&designPrefixed); err != nil {
		t.Fatal(err)
	}
	if designPrefixed != 2 {
		t.Fatalf("legacy design items=%d want 2 (promote rewrites these)", designPrefixed)
	}
}
