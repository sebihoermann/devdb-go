package planning

import (
	"database/sql"
	"fmt"

	"github.com/sebihoermann/devdb-go/internal/storage"
)

// DeletePlanInput controls a plan deletion.
type DeletePlanInput struct {
	// Force allows deleting a plan that still has in_progress items.
	Force bool
	// DryRun counts what would be deleted without writing.
	DryRun bool
}

// DeletePlanResult reports the cascade counts.
type DeletePlanResult struct {
	PlanID          string `json:"plan_id"`
	Slug            string `json:"slug"`
	Title           string `json:"title"`
	Acceptance      int    `json:"acceptance"`
	Files           int    `json:"files"`
	StatusLog       int    `json:"status_log"`
	Items           int    `json:"items"`
	Milestones      int    `json:"milestones"`
	InProgressItems int    `json:"in_progress_items"`
	DryRun          bool   `json:"dry_run"`
}

// ErrPlanInProgress blocks deleting a plan with in_progress items unless
// Force is set — an agent may be mid-task on one of them.
var ErrPlanInProgress = fmt.Errorf("plan has in_progress items (pass --force to delete anyway)")

// DeletePlan removes a plan and its whole subtree in one transaction,
// in FK-safe order: plan_item_acceptance + plan_item_files + status_log
// (children of plan_items) -> plan_items -> milestones -> plans.
// Refuses (ErrPlanInProgress) when items are in_progress unless force.
// This replaces the hand-ordered raw-SQL cascade recipe agents had to
// improvise (skill pitfall 10).
func DeletePlan(db *sql.DB, slugOrID string, in DeletePlanInput) (DeletePlanResult, error) {
	planID, err := ResolvePlanID(db, slugOrID)
	if err != nil {
		return DeletePlanResult{}, err
	}
	res := DeletePlanResult{PlanID: planID, DryRun: in.DryRun}
	if err := db.QueryRow(`SELECT COALESCE(slug,''), title FROM plans WHERE id=?`, planID).
		Scan(&res.Slug, &res.Title); err != nil {
		return res, err
	}

	countOne := func(query string, args ...any) (int, error) {
		var n int
		if err := db.QueryRow(query, args...).Scan(&n); err != nil {
			return 0, err
		}
		return n, nil
	}
	// All counts share the same child sets used by the deletes below.
	const itemIDs = `SELECT id FROM plan_items WHERE plan_id=?`
	if res.Acceptance, err = countOne(
		`SELECT COUNT(*) FROM plan_item_acceptance WHERE plan_item_id IN (`+itemIDs+`)`, planID); err != nil {
		return res, err
	}
	if res.Files, err = countOne(
		`SELECT COUNT(*) FROM plan_item_files WHERE plan_item_id IN (`+itemIDs+`)`, planID); err != nil {
		return res, err
	}
	if res.StatusLog, err = countOne(
		`SELECT COUNT(*) FROM status_log WHERE plan_item_id IN (`+itemIDs+`)`, planID); err != nil {
		return res, err
	}
	if res.Items, err = countOne(`SELECT COUNT(*) FROM plan_items WHERE plan_id=?`, planID); err != nil {
		return res, err
	}
	if res.Milestones, err = countOne(`SELECT COUNT(*) FROM milestones WHERE plan_id=?`, planID); err != nil {
		return res, err
	}
	if res.InProgressItems, err = countOne(
		`SELECT COUNT(*) FROM plan_items WHERE plan_id=? AND status='in_progress'`, planID); err != nil {
		return res, err
	}
	if res.InProgressItems > 0 && !in.Force && !in.DryRun {
		return res, ErrPlanInProgress
	}
	if in.DryRun {
		return res, nil
	}

	err = storage.WithTx(db, func(tx *sql.Tx) error {
		steps := []struct {
			query string
			args  []any
		}{
			{`DELETE FROM plan_item_acceptance WHERE plan_item_id IN (` + itemIDs + `)`, []any{planID}},
			{`DELETE FROM plan_item_files WHERE plan_item_id IN (` + itemIDs + `)`, []any{planID}},
			{`DELETE FROM status_log WHERE plan_item_id IN (` + itemIDs + `)`, []any{planID}},
			{`DELETE FROM plan_items WHERE plan_id=?`, []any{planID}},
			{`DELETE FROM milestones WHERE plan_id=?`, []any{planID}},
			{`DELETE FROM plans WHERE id=?`, []any{planID}},
		}
		for _, s := range steps {
			if _, err := tx.Exec(s.query, s.args...); err != nil {
				return fmt.Errorf("delete plan %s: %w", planID[:8], err)
			}
		}
		return nil
	})
	return res, err
}
