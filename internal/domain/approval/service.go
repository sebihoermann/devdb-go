package approval

import (
	"database/sql"
	"fmt"

	"github.com/sebihoermann/devdb-go/internal/storage"
)

// LogRow is an approval audit entry.
type LogRow struct {
	ID          string `json:"id"`
	EntityTable string `json:"entity_table"`
	EntityID    string `json:"entity_id"`
	Action      string `json:"action"`
	Note        string `json:"note,omitempty"`
	CreatedAt   string `json:"created_at"`
}

// Request marks an entity as pending approval.
func Request(db *sql.DB, table, entityID, note, modelID string) (string, error) {
	return logAction(db, table, entityID, "request", note, modelID, "pending")
}

// Approve marks an entity approved.
func Approve(db *sql.DB, table, entityID, note, modelID string) (string, error) {
	return logAction(db, table, entityID, "approve", note, modelID, "approved")
}

// Reject marks an entity rejected.
func Reject(db *sql.DB, table, entityID, note, modelID string) (string, error) {
	return logAction(db, table, entityID, "reject", note, modelID, "rejected")
}

// Withdraw clears approval state.
func Withdraw(db *sql.DB, table, entityID, note, modelID string) (string, error) {
	return logAction(db, table, entityID, "withdraw", note, modelID, "none")
}

func logAction(db *sql.DB, table, entityID, action, note, modelID, newStatus string) (string, error) {
	if table != "tasks" && table != "plan_items" {
		return "", fmt.Errorf("entity table must be tasks or plan_items")
	}
	id, err := storage.NewID()
	if err != nil {
		return "", err
	}
	now := storage.NowUTC()
	tx, err := db.Begin()
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`
		INSERT INTO approval_log(id, entity_table, entity_id, action, note, created_at, model_id)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, table, entityID, action, nullStr(note), now, modelID,
	); err != nil {
		return "", err
	}
	if _, err := tx.Exec(fmt.Sprintf(`UPDATE %s SET approval_status=? WHERE id=?`, table), newStatus, entityID); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return id, nil
}

// ListPending returns entities with pending approval.
func ListPending(db *sql.DB) ([]map[string]any, error) {
	var out []map[string]any
	for _, table := range []string{"tasks", "plan_items"} {
		rows, err := db.Query(fmt.Sprintf(`
			SELECT id, title, approval_status FROM %s WHERE approval_status='pending'`, table))
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id, title, status string
			if err := rows.Scan(&id, &title, &status); err != nil {
				rows.Close()
				return nil, err
			}
			out = append(out, map[string]any{
				"entity_table": table, "entity_id": id, "title": title, "approval_status": status,
			})
		}
		rows.Close()
	}
	return out, nil
}

// Log returns recent approval log rows. limit 0 means no cap.
func Log(db *sql.DB, limit int) ([]LogRow, error) {
	if limit < 0 {
		limit = 30
	}
	q := `SELECT id, entity_table, entity_id, action, COALESCE(note,''), created_at
		FROM approval_log ORDER BY created_at DESC`
	q, args := storage.AppendLimit(q, nil, limit)
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LogRow
	for rows.Next() {
		var r LogRow
		if err := rows.Scan(&r.ID, &r.EntityTable, &r.EntityID, &r.Action, &r.Note, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}
