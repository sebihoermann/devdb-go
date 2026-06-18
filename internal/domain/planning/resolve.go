package planning

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"github.com/sebihoermann/devdb-go/internal/storage"
)

// ResolvePlanID finds a plan by slug, id, or id prefix.
func ResolvePlanID(db *sql.DB, slugOrID string) (string, error) {
	var id string
	err := db.QueryRow(`
		SELECT id FROM plans WHERE slug=? OR id=? OR id LIKE ?`,
		slugOrID, slugOrID, slugOrID+"%",
	).Scan(&id)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("plan not found: %s", slugOrID)
	}
	return id, err
}

// ResolveMilestoneID finds a milestone within a plan by id prefix or number (e.g. "1", "M1").
func ResolveMilestoneID(db *sql.DB, planID, ref string) (string, int, error) {
	rows, err := db.Query(`SELECT id, number FROM milestones WHERE plan_id=?`, planID)
	if err != nil {
		return "", 0, err
	}
	defer rows.Close()
	var ids []string
	idToNum := map[string]int{}
	for rows.Next() {
		var id string
		var num int
		if err := rows.Scan(&id, &num); err != nil {
			return "", 0, err
		}
		ids = append(ids, id)
		idToNum[id] = num
	}
	if err := rows.Err(); err != nil {
		return "", 0, err
	}
	if id, err := storage.ResolveID(ref, ids); err == nil {
		return id, idToNum[id], nil
	}
	numRef := strings.TrimSpace(ref)
	if strings.HasPrefix(strings.ToUpper(numRef), "M") {
		numRef = numRef[1:]
	}
	n, err := strconv.Atoi(numRef)
	if err != nil {
		return "", 0, fmt.Errorf("milestone not found: %s", ref)
	}
	for id, num := range idToNum {
		if num == n {
			return id, n, nil
		}
	}
	return "", 0, fmt.Errorf("milestone not found: %s", ref)
}
