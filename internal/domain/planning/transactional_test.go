package planning_test

import (
	"testing"

	"github.com/sebihoermann/devdb-go/internal/domain/planning"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

// TestSetItemStatusRollsBackOnLogFailure verifies that when the audit-log
// insert fails (e.g. the status_log table has been dropped), the plan_items
// status update is rolled back so the item cannot end up with a status change
// that has no matching status_log row. Regression for review findings ded0572a
// and 50f1682f.
func TestSetItemStatusRollsBackOnLogFailure(t *testing.T) {
	db, _ := testutil.TempDB(t)
	planID, err := planning.CreatePlan(db, planning.CreatePlanInput{Slug: "rollback", Title: "R", ModelID: "test"})
	if err != nil {
		t.Fatal(err)
	}
	itemID, err := planning.AddItem(db, planning.AddItemInput{PlanID: planID, Title: "task", ModelID: "test"})
	if err != nil {
		t.Fatal(err)
	}

	// Drop status_log so the audit insert fails.
	if _, err := db.Exec(`DROP TABLE status_log`); err != nil {
		t.Fatal(err)
	}

	if err := planning.SetItemStatus(db, itemID, "in_progress", "should roll back", "test"); err == nil {
		t.Fatal("expected SetItemStatus to fail when status_log is missing")
	}

	// Plan item status must still be the original "planned" — the failed
	// update must have been rolled back.
	var status string
	if err := db.QueryRow(`SELECT status FROM plan_items WHERE id=?`, itemID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "planned" {
		t.Fatalf("status=%q, want planned (rollback failed)", status)
	}
}

// TestSetItemStatusExplicitRollsBackOnLogFailure covers the public path that
// resolves the prefix and delegates to SetItemStatus. The rollback guarantee
// must hold through the prefix lookup.
func TestSetItemStatusExplicitRollsBackOnLogFailure(t *testing.T) {
	db, _ := testutil.TempDB(t)
	planID, _ := planning.CreatePlan(db, planning.CreatePlanInput{Slug: "rollback-explicit", Title: "R", ModelID: "test"})
	itemID, _ := planning.AddItem(db, planning.AddItemInput{PlanID: planID, Title: "task", ModelID: "test"})

	if _, err := db.Exec(`DROP TABLE status_log`); err != nil {
		t.Fatal(err)
	}

	if _, err := planning.SetItemStatusExplicit(db, itemID[:8], "done", "should roll back", "test"); err == nil {
		t.Fatal("expected SetItemStatusExplicit to fail when status_log is missing")
	}

	var status string
	if err := db.QueryRow(`SELECT status FROM plan_items WHERE id=?`, itemID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "planned" {
		t.Fatalf("status=%q, want planned", status)
	}
}

// TestSetItemStatusHappyPathStillLogs makes sure the transactional refactor
// did not regress the common case — a successful status change still produces
// a matching status_log row.
func TestSetItemStatusHappyPathStillLogs(t *testing.T) {
	db, _ := testutil.TempDB(t)
	planID, _ := planning.CreatePlan(db, planning.CreatePlanInput{Slug: "happy", Title: "H", ModelID: "test"})
	itemID, _ := planning.AddItem(db, planning.AddItemInput{PlanID: planID, Title: "task", ModelID: "test"})

	if err := planning.SetItemStatus(db, itemID, "in_progress", "started", "test"); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM status_log WHERE plan_item_id=?`, itemID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("status_log rows=%d, want 1", n)
	}
	var status string
	if err := db.QueryRow(`SELECT status FROM plan_items WHERE id=?`, itemID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "in_progress" {
		t.Fatalf("status=%q, want in_progress", status)
	}
}

// TestSetItemStatusTriggerForcesRollback installs a BEFORE INSERT trigger on
// status_log that always raises ABORT. The SetItemStatus call must roll back
// the plan_items update when the trigger fires, mirroring any constraint-style
// failure. Regression for review findings ded0572a and 50f1682f.
func TestSetItemStatusTriggerForcesRollback(t *testing.T) {
	db, _ := testutil.TempDB(t)
	planID, _ := planning.CreatePlan(db, planning.CreatePlanInput{Slug: "trigger", Title: "T", ModelID: "test"})
	itemID, _ := planning.AddItem(db, planning.AddItemInput{PlanID: planID, Title: "task", ModelID: "test"})

	if _, err := db.Exec(`
		CREATE TRIGGER status_log_block_insert
		BEFORE INSERT ON status_log
		BEGIN
			SELECT RAISE(ABORT, 'forced log failure');
		END`); err != nil {
		t.Fatal(err)
	}

	if err := planning.SetItemStatus(db, itemID, "done", "trigger test", "test"); err == nil {
		t.Fatal("expected SetItemStatus to fail when status_log trigger raises")
	}
	var status string
	if err := db.QueryRow(`SELECT status FROM plan_items WHERE id=?`, itemID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "planned" {
		t.Fatalf("status=%q, want planned (trigger rollback failed)", status)
	}
}