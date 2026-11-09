package planning

import (
	"database/sql"
	"strings"
)

// BlockedItem is a plan item that is in progress and whose most recent
// in-progress status note reports a blocker.
type BlockedItem struct {
	// PlanItem is the item the note belongs to.
	PlanItem PlanItem
	// Note is the status note that reported the blocker.
	Note string
	// Since is when that note was recorded.
	Since string
}

// blockedKeywords are the substrings that mark a status note as "waiting on
// something".
//
// The cross-project audit in internal/domain/hub/audit.go carries a parallel
// copy of this list. Two copies drift: a keyword added here is invisible to
// the fleet audit and vice versa, so both views of "blocked" disagree. The
// duplication is deliberate for now — consolidating it means changing the
// audit's SQL, which has its own regression coverage — but it is debt, and
// the next person to touch either list should note it.
var blockedKeywords = []string{
	"blocked", "blocker", "waiting", "stuck", "cannot proceed", "can't proceed",
}

// ListBlocked returns the plan items that are in progress and whose latest
// in-progress status note reports a blocker.
//
// Only the most recent in-progress note counts. An item that was blocked
// last week and has since received a different note is no longer blocked,
// and the audit trail must not resurrect it. The tiebreak is on row id as
// well as timestamp, so two notes written in the same millisecond resolve to
// exactly one item instead of duplicating it.
func ListBlocked(db *sql.DB) ([]BlockedItem, error) {
	clauses := make([]string, 0, len(blockedKeywords))
	args := []any{}
	for _, kw := range blockedKeywords {
		clauses = append(clauses, "LOWER(sl.note) LIKE ?")
		args = append(args, "%"+strings.ToLower(kw)+"%")
	}
	q := `
		SELECT pi.id, COALESCE(pi.plan_id,''), COALESCE(pi.milestone_id,''), COALESCE(pi.item_number,0),
		       pi.title, COALESCE(pi.body,''), pi.status, pi.approval_status, pi.created_at,
		       COALESCE(pi.phase,''), COALESCE(pi.step,''), COALESCE(pi.memory_ref,''),
		       COALESCE(sl.note,''), sl.created_at
		FROM plan_items pi
		JOIN status_log sl ON sl.plan_item_id = pi.id
		WHERE pi.status = 'in_progress'
		  AND sl.status = 'in_progress'
		  AND sl.id = (
		    SELECT sl3.id FROM status_log sl3
		    WHERE sl3.plan_item_id = pi.id AND sl3.status = 'in_progress'
		    ORDER BY sl3.created_at DESC, sl3.id DESC
		    LIMIT 1
		  )
		  AND (` + strings.Join(clauses, " OR ") + `)
		ORDER BY sl.created_at DESC`
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BlockedItem
	for rows.Next() {
		var b BlockedItem
		if err := rows.Scan(&b.PlanItem.ID, &b.PlanItem.PlanID, &b.PlanItem.MilestoneID,
			&b.PlanItem.ItemNumber, &b.PlanItem.Title, &b.PlanItem.Body, &b.PlanItem.Status,
			&b.PlanItem.Approval, &b.PlanItem.CreatedAt, &b.PlanItem.Phase, &b.PlanItem.Step,
			&b.PlanItem.MemoryRef, &b.Note, &b.Since); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
