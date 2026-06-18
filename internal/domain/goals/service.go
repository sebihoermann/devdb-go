package goals

import (
	"database/sql"
	"fmt"

	"github.com/sebihoermann/devdb-go/internal/storage"
)

// Row is a goal ledger entry.
type Row struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Title     string `json:"title"`
	Body      string `json:"body,omitempty"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

// Add creates a goal and returns its id.
func Add(db *sql.DB, kind, title, body, modelID string) (string, error) {
	if kind != "goal" && kind != "do" && kind != "dont" {
		return "", fmt.Errorf("kind must be goal, do, or dont")
	}
	id, err := storage.NewID()
	if err != nil {
		return "", err
	}
	now := storage.NowUTC()
	_, err = db.Exec(`
		INSERT INTO goals(id, kind, title, body, status, created_at, model_id)
		VALUES (?, ?, ?, ?, 'active', ?, ?)`,
		id, kind, title, nullIfEmpty(body), now, modelID,
	)
	return id, err
}

// List returns goals filtered by status. limit 0 means no cap.
func List(db *sql.DB, status string, limit int) ([]Row, error) {
	if limit < 0 {
		limit = 20
	}
	q := `SELECT id, kind, title, COALESCE(body,''), status, created_at FROM goals`
	args := []any{}
	if status != "" && status != "all" {
		q += ` WHERE status = ?`
		args = append(args, status)
	}
	q += ` ORDER BY created_at DESC`
	q, args = storage.AppendLimit(q, args, limit)
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Row
	for rows.Next() {
		var r Row
		if err := rows.Scan(&r.ID, &r.Kind, &r.Title, &r.Body, &r.Status, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// SetStatus updates goal status.
func SetStatus(db *sql.DB, idPrefix, status, modelID string) (string, error) {
	if status != "active" && status != "done" && status != "wontfix" {
		return "", fmt.Errorf("status must be active, done, or wontfix")
	}
	id, err := resolveID(db, idPrefix)
	if err != nil {
		return "", err
	}
	_, err = db.Exec(`UPDATE goals SET status=? WHERE id=?`, status, id)
	return id, err
}

func resolveID(db *sql.DB, prefix string) (string, error) {
	rows, err := db.Query(`SELECT id FROM goals`)
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

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
