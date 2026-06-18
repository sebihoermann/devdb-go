package planning

import (
	"database/sql"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/sebihoermann/devdb-go/internal/storage"
)

// BackfillAcceptanceFromSpec parses a markdown spec and adds acceptance rows for a milestone.
func BackfillAcceptanceFromSpec(db *sql.DB, milestone, specPath, modelID string) (int, error) {
	data, err := os.ReadFile(specPath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, fmt.Errorf("spec file not found: %s", specPath)
		}
		return 0, err
	}
	specText := string(data)

	milestonePattern := regexp.MustCompile(`(?m)^### ` + regexp.QuoteMeta(milestone) + ` `)
	match := milestonePattern.FindStringIndex(specText)
	if match == nil {
		return 0, nil
	}
	remaining := specText[match[1]:]
	nextMilestone := regexp.MustCompile(`(?m)^### [A-Z]`)
	nextMatch := nextMilestone.FindStringIndex(remaining)
	sectionEnd := len(remaining)
	if nextMatch != nil {
		sectionEnd = nextMatch[0]
	}
	section := specText[match[0]:match[1]+sectionEnd]

	acceptancePattern := regexp.MustCompile(`(?m)^- \[[xX ]\] (.+)$`)
	acceptances := acceptancePattern.FindAllStringSubmatch(section, -1)
	if len(acceptances) == 0 {
		return 0, nil
	}

	step := strings.ToLower(milestone)
	var planItemID string
	err = db.QueryRow(`SELECT id FROM plan_items WHERE step=?`, step).Scan(&planItemID)
	if err == sql.ErrNoRows {
		planItemID, err = storage.NewID()
		if err != nil {
			return 0, err
		}
		now := storage.NowUTC()
		_, err = db.Exec(`
			INSERT INTO plan_items (id, phase, step, title, body, status, created_at, model_id)
			VALUES (?, 'milestones', ?, ?, 'See SPEC.md', 'planned', ?, ?)`,
			planItemID, step, milestone, now, modelID,
		)
		if err != nil {
			return 0, err
		}
	} else if err != nil {
		return 0, err
	}

	count := 0
	for ordinal, m := range acceptances {
		criterion := strings.TrimSpace(m[1])
		var existing string
		err := db.QueryRow(`
			SELECT id FROM plan_item_acceptance WHERE plan_item_id=? AND ordinal=?`,
			planItemID, ordinal+1).Scan(&existing)
		if err == sql.ErrNoRows {
			accID, err := storage.NewID()
			if err != nil {
				return count, err
			}
			now := storage.NowUTC()
			if _, err := db.Exec(`
				INSERT INTO plan_item_acceptance
				(id, plan_item_id, ordinal, criterion, status, created_at, updated_at, model_id)
				VALUES (?, ?, ?, ?, 'open', ?, ?, ?)`,
				accID, planItemID, ordinal+1, criterion, now, now, modelID); err != nil {
				return count, err
			}
			count++
		} else if err != nil {
			return count, err
		}
	}
	return count, nil
}
