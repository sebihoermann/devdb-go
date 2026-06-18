package planning

import (
	"database/sql"
)

type planQuerier interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

// MilestoneDrift is a milestone whose status does not match its children.
type MilestoneDrift struct {
	ID             string `json:"id"`
	PlanSlug       string `json:"plan_slug"`
	Number         int    `json:"number"`
	Title          string `json:"title"`
	CurrentStatus  string `json:"current_status"`
	ExpectedStatus string `json:"expected_status"`
}

// PlanDrift is a plan whose status does not match its milestones.
type PlanDrift struct {
	ID             string `json:"id"`
	Slug           string `json:"slug"`
	Title          string `json:"title"`
	CurrentStatus  string `json:"current_status"`
	ExpectedStatus string `json:"expected_status"`
}

// DriftReport lists plan-tree status mismatches.
type DriftReport struct {
	Milestones []MilestoneDrift `json:"milestones"`
	Plans      []PlanDrift      `json:"plans"`
}

// AppliedReconcile records rows repaired during reconcile.
type AppliedReconcile struct {
	Milestones []map[string]any `json:"milestones"`
	Plans      []map[string]any `json:"plans"`
}

// ReconcileResult is the output of reconcile (detect or repair).
type ReconcileResult struct {
	Drift   DriftReport       `json:"drift"`
	Applied *AppliedReconcile `json:"applied"`
}

func milestoneStatusFromChildren(q planQuerier, milestoneID string) (string, error) {
	rows, err := q.Query(`
		SELECT status, COUNT(*) FROM plan_items WHERE milestone_id=? GROUP BY status`, milestoneID)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	counts := map[string]int{}
	total := 0
	for rows.Next() {
		var status string
		var c int
		if err := rows.Scan(&status, &c); err != nil {
			return "", err
		}
		counts[status] = c
		total += c
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if total == 0 {
		var parentStatus string
		err := q.QueryRow(`
			SELECT p.status FROM plans p
			JOIN milestones m ON m.plan_id = p.id
			WHERE m.id = ?`, milestoneID).Scan(&parentStatus)
		if err == nil && parentStatus == "done" {
			return "done", nil
		}
		return "", nil
	}
	if counts["in_progress"] > 0 {
		return "in_progress", nil
	}
	if counts["planned"] > 0 {
		return "planned", nil
	}
	if counts["done"] > 0 {
		return "done", nil
	}
	if counts["wontfix"] > 0 {
		return "wontfix", nil
	}
	return "", nil
}

func planStatusFromMilestones(q planQuerier, planID string) (string, error) {
	rows, err := q.Query(`SELECT status, COUNT(*) FROM milestones WHERE plan_id=? GROUP BY status`, planID)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	counts := map[string]int{}
	total := 0
	for rows.Next() {
		var status string
		var c int
		if err := rows.Scan(&status, &c); err != nil {
			return "", err
		}
		counts[status] = c
		total += c
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if total == 0 {
		return "", nil
	}
	if counts["in_progress"] > 0 || counts["planned"] > 0 {
		return "active", nil
	}
	if counts["done"] > 0 || counts["wontfix"] > 0 {
		return "done", nil
	}
	return "", nil
}

// FindPlanTreeDrift returns milestones and plans whose status does not match children.
func FindPlanTreeDrift(db *sql.DB, planRef string) (DriftReport, error) {
	var planRows []struct {
		id, slug, title, status string
	}
	if planRef != "" {
		planID, err := ResolvePlanID(db, planRef)
		if err != nil {
			return DriftReport{}, err
		}
		var p struct {
			id, slug, title, status string
		}
		if err := db.QueryRow(`SELECT id, slug, title, status FROM plans WHERE id=?`, planID).
			Scan(&p.id, &p.slug, &p.title, &p.status); err != nil {
			return DriftReport{}, err
		}
		planRows = append(planRows, p)
	} else {
		rows, err := db.Query(`SELECT id, slug, title, status FROM plans ORDER BY created_at`)
		if err != nil {
			return DriftReport{}, err
		}
		defer rows.Close()
		for rows.Next() {
			var p struct {
				id, slug, title, status string
			}
			if err := rows.Scan(&p.id, &p.slug, &p.title, &p.status); err != nil {
				return DriftReport{}, err
			}
			planRows = append(planRows, p)
		}
		if err := rows.Err(); err != nil {
			return DriftReport{}, err
		}
	}

	report := DriftReport{}
	for _, plan := range planRows {
		msRows, err := db.Query(`
			SELECT id, number, title, status FROM milestones WHERE plan_id=? ORDER BY number`, plan.id)
		if err != nil {
			return DriftReport{}, err
		}
		type msRow struct {
			id, title, status string
			number            int
		}
		var milestones []msRow
		for msRows.Next() {
			var ms msRow
			if err := msRows.Scan(&ms.id, &ms.number, &ms.title, &ms.status); err != nil {
				msRows.Close()
				return DriftReport{}, err
			}
			milestones = append(milestones, ms)
		}
		if err := msRows.Close(); err != nil {
			return DriftReport{}, err
		}
		if err := msRows.Err(); err != nil {
			return DriftReport{}, err
		}

		for _, ms := range milestones {
			expected, err := milestoneStatusFromChildren(db, ms.id)
			if err != nil {
				return DriftReport{}, err
			}
			if expected != "" && expected != ms.status {
				report.Milestones = append(report.Milestones, MilestoneDrift{
					ID: ms.id, PlanSlug: plan.slug, Number: ms.number, Title: ms.title,
					CurrentStatus: ms.status, ExpectedStatus: expected,
				})
			}
		}

		expectedPlan, err := planStatusFromMilestones(db, plan.id)
		if err != nil {
			return DriftReport{}, err
		}
		if expectedPlan != "" && expectedPlan != plan.status {
			report.Plans = append(report.Plans, PlanDrift{
				ID: plan.id, Slug: plan.slug, Title: plan.title,
				CurrentStatus: plan.status, ExpectedStatus: expectedPlan,
			})
		}
	}
	return report, nil
}

func syncMilestoneStatus(q planQuerier, milestoneID string) (string, error) {
	status, err := milestoneStatusFromChildren(q, milestoneID)
	if err != nil || status == "" {
		return "", err
	}
	if _, err := q.Exec(`UPDATE milestones SET status=? WHERE id=?`, status, milestoneID); err != nil {
		return "", err
	}
	return status, nil
}

func syncPlanForMilestone(q planQuerier, milestoneID string) (string, error) {
	var planID string
	if err := q.QueryRow(`SELECT plan_id FROM milestones WHERE id=?`, milestoneID).Scan(&planID); err != nil {
		return "", err
	}
	return syncPlanStatus(q, planID)
}

func syncPlanStatus(q planQuerier, planID string) (string, error) {
	status, err := planStatusFromMilestones(q, planID)
	if err != nil || status == "" {
		return "", err
	}
	if _, err := q.Exec(`UPDATE plans SET status=? WHERE id=?`, status, planID); err != nil {
		return "", err
	}
	return status, nil
}

// ReconcilePlans detects or repairs plan-tree status drift.
func ReconcilePlans(db *sql.DB, planRef string, apply bool) (ReconcileResult, error) {
	drift, err := FindPlanTreeDrift(db, planRef)
	if err != nil {
		return ReconcileResult{}, err
	}
	if !apply {
		return ReconcileResult{Drift: drift, Applied: nil}, nil
	}

	tx, err := db.Begin()
	if err != nil {
		return ReconcileResult{}, err
	}
	defer func() { _ = tx.Rollback() }()

	applied := &AppliedReconcile{}
	for _, item := range drift.Milestones {
		newStatus, err := syncMilestoneStatus(tx, item.ID)
		if err != nil {
			return ReconcileResult{}, err
		}
		applied.Milestones = append(applied.Milestones, map[string]any{
			"id": item.ID, "plan_slug": item.PlanSlug, "number": item.Number,
			"current_status": item.CurrentStatus, "expected_status": item.ExpectedStatus,
			"new_status": newStatus,
		})
		if _, err := syncPlanForMilestone(tx, item.ID); err != nil {
			return ReconcileResult{}, err
		}
	}
	for _, item := range drift.Plans {
		newStatus, err := syncPlanStatus(tx, item.ID)
		if err != nil {
			return ReconcileResult{}, err
		}
		applied.Plans = append(applied.Plans, map[string]any{
			"id": item.ID, "slug": item.Slug,
			"current_status": item.CurrentStatus, "expected_status": item.ExpectedStatus,
			"new_status": newStatus,
		})
	}
	if err := tx.Commit(); err != nil {
		return ReconcileResult{}, err
	}
	return ReconcileResult{Drift: drift, Applied: applied}, nil
}

// DriftCount returns total drifted rows.
func (d DriftReport) DriftCount() int {
	return len(d.Milestones) + len(d.Plans)
}

// String summary for empty drift check.
func (d DriftReport) IsEmpty() bool {
	return len(d.Milestones) == 0 && len(d.Plans) == 0
}