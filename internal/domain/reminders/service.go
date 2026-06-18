package reminders

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/sebihoermann/devdb-go/internal/storage"
)

// Row is a reminder entry.
type Row struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Body       string `json:"body,omitempty"`
	DueAt      string `json:"due_at,omitempty"`
	FilePath   string `json:"file_path,omitempty"`
	PlanItemID string `json:"plan_item_id,omitempty"`
	Status     string `json:"status"`
	SnoozeUntil string `json:"snooze_until,omitempty"`
	CreatedAt  string `json:"created_at"`
}

// AddInput for new reminders.
type AddInput struct {
	Title      string
	Body       string
	DueAt      string
	FilePath   string
	PlanItemID string
	ModelID    string
}

// Add creates a reminder.
func Add(db *sql.DB, in AddInput) (string, error) {
	id, err := storage.NewID()
	if err != nil {
		return "", err
	}
	now := storage.NowUTC()
	_, err = db.Exec(`
		INSERT INTO reminders(id, title, body, due_at, file_path, plan_item_id, status, created_at, model_id)
		VALUES (?, ?, ?, ?, ?, ?, 'open', ?, ?)`,
		id, in.Title, nullStr(in.Body), nullStr(in.DueAt), nullStr(in.FilePath),
		nullStr(in.PlanItemID), now, in.ModelID,
	)
	return id, err
}

// List returns reminders. limit 0 means no cap.
func List(db *sql.DB, status string, overdueOnly bool, limit int) ([]Row, error) {
	if limit < 0 {
		limit = 30
	}
	q := `SELECT id, title, COALESCE(body,''), COALESCE(due_at,''), COALESCE(file_path,''),
	      COALESCE(plan_item_id,''), status, COALESCE(snooze_until,''), created_at
	      FROM reminders WHERE 1=1`
	args := []any{}
	if status != "" && status != "all" {
		q += ` AND status = ?`
		args = append(args, status)
	}
	if overdueOnly {
		q += ` AND status='open' AND due_at IS NOT NULL AND due_at < datetime('now')`
	}
	q += ` ORDER BY due_at IS NULL, due_at ASC`
	q, args = storage.AppendLimit(q, args, limit)
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Row
	for rows.Next() {
		var r Row
		if err := rows.Scan(&r.ID, &r.Title, &r.Body, &r.DueAt, &r.FilePath, &r.PlanItemID, &r.Status, &r.SnoozeUntil, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Show returns one reminder.
func Show(db *sql.DB, idPrefix string) (Row, error) {
	id, err := resolveID(db, idPrefix)
	if err != nil {
		return Row{}, err
	}
	var r Row
	err = db.QueryRow(`
		SELECT id, title, COALESCE(body,''), COALESCE(due_at,''), COALESCE(file_path,''),
		       COALESCE(plan_item_id,''), status, COALESCE(snooze_until,''), created_at
		FROM reminders WHERE id=?`, id,
	).Scan(&r.ID, &r.Title, &r.Body, &r.DueAt, &r.FilePath, &r.PlanItemID, &r.Status, &r.SnoozeUntil, &r.CreatedAt)
	return r, err
}

// Dismiss closes a reminder.
func Dismiss(db *sql.DB, idPrefix string) (string, error) {
	id, err := resolveID(db, idPrefix)
	if err != nil {
		return "", err
	}
	_, err = db.Exec(`UPDATE reminders SET status='dismissed' WHERE id=?`, id)
	return id, err
}

// Snooze sets snooze_until.
func Snooze(db *sql.DB, idPrefix, until string) (string, error) {
	if strings.TrimSpace(until) == "" {
		return "", fmt.Errorf("--until is required")
	}
	id, err := resolveID(db, idPrefix)
	if err != nil {
		return "", err
	}
	_, err = db.Exec(`UPDATE reminders SET snooze_until=? WHERE id=?`, until, id)
	return id, err
}

// Unsnooze clears snooze_until.
func Unsnooze(db *sql.DB, idPrefix string) (string, error) {
	id, err := resolveID(db, idPrefix)
	if err != nil {
		return "", err
	}
	_, err = db.Exec(`UPDATE reminders SET snooze_until=NULL WHERE id=?`, id)
	return id, err
}

func resolveID(db *sql.DB, prefix string) (string, error) {
	rows, err := db.Query(`SELECT id FROM reminders`)
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
	return storage.ResolveID(prefix, ids)
}

func nullStr(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

// ListForFile returns open reminders tagged to a file path.
func ListForFile(db *sql.DB, filePath string) ([]Row, error) {
	rows, err := db.Query(`
		SELECT id, title, COALESCE(body,''), COALESCE(due_at,''), COALESCE(file_path,''),
		       COALESCE(plan_item_id,''), status, COALESCE(snooze_until,''), created_at
		FROM reminders WHERE status='open' AND file_path=? ORDER BY due_at IS NULL, due_at ASC`, filePath)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanReminderRows(rows)
}

// ListForPlanItem returns open reminders linked to a plan item.
func ListForPlanItem(db *sql.DB, planItemID string) ([]Row, error) {
	rows, err := db.Query(`
		SELECT id, title, COALESCE(body,''), COALESCE(due_at,''), COALESCE(file_path,''),
		       COALESCE(plan_item_id,''), status, COALESCE(snooze_until,''), created_at
		FROM reminders WHERE status='open' AND plan_item_id=? ORDER BY due_at IS NULL, due_at ASC`, planItemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanReminderRows(rows)
}

// IsOverdue reports whether a reminder is past due and still open.
func IsOverdue(r Row) bool {
	if r.Status != "open" || r.DueAt == "" {
		return false
	}
	if r.SnoozeUntil != "" && r.SnoozeUntil > storage.NowUTC() {
		return false
	}
	return r.DueAt < storage.NowUTC()
}

func scanReminderRows(rows *sql.Rows) ([]Row, error) {
	var out []Row
	for rows.Next() {
		var r Row
		if err := rows.Scan(&r.ID, &r.Title, &r.Body, &r.DueAt, &r.FilePath, &r.PlanItemID, &r.Status, &r.SnoozeUntil, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
