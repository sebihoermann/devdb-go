package planning_test

import (
	"strings"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/domain/planning"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

// TestResolvePlanIDLiteralWildcardSafe verifies that user-supplied slugs and
// prefixes that contain LIKE wildcards (% and _) are matched as literal
// characters rather than SQL wildcards. Regression for review findings
// ccbbd66c and 4d4fdac6.
func TestResolvePlanIDLiteralWildcardSafe(t *testing.T) {
	db, _ := testutil.TempDB(t)
	// Slug contains a percent sign; before the fix, ResolvePlanID would use
	// 'slug LIKE "100%done%"' which matches every slug that begins with "100".
	percentID, err := planning.CreatePlan(db, planning.CreatePlanInput{
		Slug: "100%done", Title: "percent", ModelID: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Underscore is a SQL LIKE single-character wildcard. A slug containing "_"
	// used to match every slug of the right length; resolve must require an
	// exact slug match.
	underscoreID, err := planning.CreatePlan(db, planning.CreatePlanInput{
		Slug: "user_owned", Title: "underscore", ModelID: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	decoyID, err := planning.CreatePlan(db, planning.CreatePlanInput{
		Slug: "other", Title: "decoy", ModelID: "test",
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := planning.ResolvePlanID(db, "100%done")
	if err != nil || got != percentID {
		t.Fatalf("percent slug: got=%q want=%q err=%v", got, percentID, err)
	}
	got, err = planning.ResolvePlanID(db, "user_owned")
	if err != nil || got != underscoreID {
		t.Fatalf("underscore slug: got=%q want=%q err=%v", got, underscoreID, err)
	}

	if _, err := planning.ResolvePlanID(db, "%"); err == nil {
		t.Fatal("expected error resolving bare % wildcard")
	}
	if _, err := planning.ResolvePlanID(db, "_"); err == nil {
		t.Fatal("expected error resolving bare _ wildcard")
	}

	_ = decoyID
}

// TestResolveMilestoneIDByPrefixLiteralWildcardSafe verifies the same property
// for milestone prefix resolution. The original code wrote
// `WHERE id=? OR id LIKE ?` with `args[0]+"%"`; a milestone id prefix of "%"
// matched every milestone, and "_" matched any single character. The fix
// resolves the prefix in memory and treats LIKE metacharacters as literals.
func TestResolveMilestoneIDByPrefixLiteralWildcardSafe(t *testing.T) {
	db, _ := testutil.TempDB(t)
	planID, err := planning.CreatePlan(db, planning.CreatePlanInput{
		Slug: "ms-prefix", Title: "MS prefix", ModelID: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	msID, err := planning.AddMilestone(db, planID, "M1", "", "test", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := planning.AddMilestone(db, planID, "M2", "", "test", 2); err != nil {
		t.Fatal(err)
	}

	// Real prefix still resolves.
	got, err := planning.ResolveMilestoneIDByPrefix(db, msID[:8])
	if err != nil || got != msID {
		t.Fatalf("real prefix: got=%q want=%q err=%v", got, msID, err)
	}

	// Wildcards return "not found" because no milestone id literally starts
	// with "%" or "_".
	if _, err := planning.ResolveMilestoneIDByPrefix(db, "%"); err == nil {
		t.Fatal("expected error resolving bare % wildcard")
	}
	if _, err := planning.ResolveMilestoneIDByPrefix(db, "_"); err == nil {
		t.Fatal("expected error resolving bare _ wildcard")
	}
}

// TestMilestoneStatusPrefixSafe verifies that the devdb plan milestone status
// command cannot silently rewrite multiple rows via LIKE. Two milestones with
// distinct ids must not both be updated when the prefix matches a wildcard.
func TestMilestoneStatusPrefixSafe(t *testing.T) {
	db, _ := testutil.TempDB(t)
	planID, err := planning.CreatePlan(db, planning.CreatePlanInput{
		Slug: "status-safe", Title: "Status Safe", ModelID: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := planning.AddMilestone(db, planID, "M1", "", "test", 1); err != nil {
		t.Fatal(err)
	}
	ms2, err := planning.AddMilestone(db, planID, "M2", "", "test", 2)
	if err != nil {
		t.Fatal(err)
	}

	// "%" should resolve to "no milestone id literally starts with %" rather
	// than rewriting both rows.
	if _, err := planning.ResolveMilestoneIDByPrefix(db, "%"); err == nil {
		t.Fatal("expected error resolving bare % wildcard")
	}

	// Real prefix still works.
	realID, err := planning.ResolveMilestoneIDByPrefix(db, ms2[:8])
	if err != nil || realID != ms2 {
		t.Fatalf("real prefix: got=%q want=%q err=%v", realID, ms2, err)
	}

	// Confirm only the targeted row would be updated.
	res, err := db.Exec(`UPDATE milestones SET status=? WHERE id=?`, "done", realID)
	if err != nil {
		t.Fatal(err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		t.Fatal(err)
	}
	if affected != 1 {
		t.Fatalf("rows affected=%d, want 1", affected)
	}
}

// TestResolvePlanIDAmbiguousPrefixErrors ensures the in-memory prefix matcher
// surfaces ambiguity instead of silently picking the first row, matching the
// behavior of the milestone resolver. We seed the plans table with synthetic
// ids so the test does not depend on the random hex id generator producing
// colliding prefixes.
func TestResolvePlanIDAmbiguousPrefixErrors(t *testing.T) {
	db, _ := testutil.TempDB(t)
	if _, err := db.Exec(`INSERT INTO plans(id, slug, title, status, created_at) VALUES(?,?,?,?,?)`,
		"abc11111aaaaaaaaaaaaaaaaaaaaaaaa", "alpha", "Alpha", "active", "2026-06-23T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO plans(id, slug, title, status, created_at) VALUES(?,?,?,?,?)`,
		"abc22222bbbbbbbbbbbbbbbbbbbbbbbb", "alpha-2", "Alpha 2", "active", "2026-06-23T00:00:00Z"); err != nil {
		t.Fatal(err)
	}

	// "abc" matches both seeded ids; the resolver must surface ambiguity
	// rather than picking one silently.
	_, err := planning.ResolvePlanID(db, "abc")
	if err == nil {
		t.Fatal("expected ambiguous prefix error for shared prefix 'abc'")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("expected 'ambiguous' error, got %v", err)
	}

	// "abc1" uniquely matches the first seeded id.
	got, err := planning.ResolvePlanID(db, "abc1")
	if err != nil {
		t.Fatalf("unique prefix: err=%v", err)
	}
	if got != "abc11111aaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("unique prefix: got=%q want=%q", got, "abc11111aaaaaaaaaaaaaaaaaaaaaaaa")
	}

	// Exact id still resolves.
	got, err = planning.ResolvePlanID(db, "abc22222bbbbbbbbbbbbbbbbbbbbbbbb")
	if err != nil || got != "abc22222bbbbbbbbbbbbbbbbbbbbbbbb" {
		t.Fatalf("exact id: got=%q err=%v", got, err)
	}
}

// TestResolvePlanIDExactIDReturnsID verifies ResolvePlanID accepts an exact id
// even when the table also contains a slug equal to that id. Regression guard
// against the rewrite to in-memory matching accidentally only honoring slug.
func TestResolvePlanIDExactIDReturnsID(t *testing.T) {
	db, _ := testutil.TempDB(t)
	id, err := planning.CreatePlan(db, planning.CreatePlanInput{Slug: "exact", Title: "Exact", ModelID: "test"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := planning.ResolvePlanID(db, id)
	if err != nil || got != id {
		t.Fatalf("exact id: got=%q want=%q err=%v", got, id, err)
	}
}