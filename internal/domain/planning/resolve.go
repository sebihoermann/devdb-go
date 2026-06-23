package planning

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"github.com/sebihoermann/devdb-go/internal/storage"
)

// ResolvePlanID finds a plan by exact slug, exact id, or unique id prefix.
// User-supplied slugs and id prefixes are matched as literal text — SQL LIKE
// wildcards ('%', '_') are not interpreted, so a slug like "100%done" or
// "user_owned" resolves the same way it reads.
func ResolvePlanID(db *sql.DB, slugOrID string) (string, error) {
	slugOrID = strings.TrimSpace(slugOrID)
	if slugOrID == "" {
		return "", fmt.Errorf("%w: empty ref", ErrPlanNotFound)
	}
	rows, err := db.Query(`SELECT id, COALESCE(slug,'') FROM plans`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var ids []string
	slugToID := map[string]string{}
	for rows.Next() {
		var id, slug string
		if err := rows.Scan(&id, &slug); err != nil {
			return "", err
		}
		ids = append(ids, id)
		if slug != "" {
			slugToID[slug] = id
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if id, ok := slugToID[slugOrID]; ok {
		return id, nil
	}
	for _, id := range ids {
		if id == slugOrID {
			return id, nil
		}
	}
	id, err := storage.ResolveID(slugOrID, ids)
	if err == nil {
		return id, nil
	}
	if strings.Contains(err.Error(), "ambiguous") {
		return "", err
	}
	return "", fmt.Errorf("%w: %s", ErrPlanNotFound, slugOrID)
}

// ResolveMilestoneIDByPrefix finds a milestone by unique id prefix across all plans.
// Returns an error if no milestone matches or if the prefix matches multiple
// milestones (ambiguous). User input is treated as literal text — LIKE
// wildcards are never interpreted.
func ResolveMilestoneIDByPrefix(db *sql.DB, prefix string) (string, error) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return "", fmt.Errorf("%w: empty id", ErrMilestoneNotFound)
	}
	rows, err := db.Query(`SELECT id FROM milestones`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return "", err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return storage.ResolveID(prefix, ids)
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
		return "", 0, fmt.Errorf("%w: %s", ErrMilestoneNotFound, ref)
	}
	for id, num := range idToNum {
		if num == n {
			return id, n, nil
		}
	}
	return "", 0, fmt.Errorf("%w: %s", ErrMilestoneNotFound, ref)
}