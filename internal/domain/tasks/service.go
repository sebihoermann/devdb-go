package tasks

import (
	"database/sql"
	"fmt"

	"github.com/sebihoermann/devdb-go/internal/storage"
)

// Row is a task ledger entry.
type Row struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	Body           string `json:"body,omitempty"`
	Status         string `json:"status"`
	Priority       string `json:"priority"`
	DueAt          string `json:"due_at,omitempty"`
	ApprovalStatus string `json:"approval_status"`
	CreatedAt      string `json:"created_at"`
}

// Add creates a task.
func Add(db *sql.DB, title, body, priority, dueAt, modelID string) (string, error) {
	if priority == "" {
		priority = "med"
	}
	id, err := storage.NewID()
	if err != nil {
		return "", err
	}
	now := storage.NowUTC()
	_, err = db.Exec(`
		INSERT INTO tasks(id, title, body, status, priority, due_at, created_at, model_id)
		VALUES (?, ?, ?, 'open', ?, ?, ?, ?)`,
		id, title, nullStr(body), priority, nullStr(dueAt), now, modelID,
	)
	return id, err
}

// List returns tasks. limit 0 means no cap.
func List(db *sql.DB, status, priority string, limit int) ([]Row, error) {
	if limit < 0 {
		limit = 50
	}
	q := `SELECT id, title, COALESCE(body,''), status, priority, COALESCE(due_at,''), approval_status, created_at FROM tasks WHERE 1=1`
	args := []any{}
	if status != "" && status != "all" {
		q += ` AND status = ?`
		args = append(args, status)
	}
	if priority != "" {
		q += ` AND priority = ?`
		args = append(args, priority)
	}
	q += ` ORDER BY CASE priority WHEN 'high' THEN 0 WHEN 'med' THEN 1 ELSE 2 END, created_at DESC`
	q, args = storage.AppendLimit(q, args, limit)
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Row
	for rows.Next() {
		var r Row
		if err := rows.Scan(&r.ID, &r.Title, &r.Body, &r.Status, &r.Priority, &r.DueAt, &r.ApprovalStatus, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Show returns one task.
func Show(db *sql.DB, idPrefix string) (Row, error) {
	id, err := resolveID(db, idPrefix)
	if err != nil {
		return Row{}, err
	}
	var r Row
	err = db.QueryRow(`
		SELECT id, title, COALESCE(body,''), status, priority, COALESCE(due_at,''), approval_status, created_at
		FROM tasks WHERE id=?`, id,
	).Scan(&r.ID, &r.Title, &r.Body, &r.Status, &r.Priority, &r.DueAt, &r.ApprovalStatus, &r.CreatedAt)
	return r, err
}

// SetStatus updates task status.
func SetStatus(db *sql.DB, idPrefix, status, modelID string) (string, error) {
	if status != "open" && status != "done" && status != "wontfix" {
		return "", fmt.Errorf("status must be open, done, or wontfix")
	}
	id, err := resolveID(db, idPrefix)
	if err != nil {
		return "", err
	}
	_, err = db.Exec(`UPDATE tasks SET status=? WHERE id=?`, status, id)
	return id, err
}

func resolveID(db *sql.DB, prefix string) (string, error) {
	rows, err := db.Query(`SELECT id FROM tasks`)
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
	if s == "" {
		return nil
	}
	return s
}
