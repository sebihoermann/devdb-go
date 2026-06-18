package features

import (
	"database/sql"

	"github.com/sebihoermann/devdb-go/internal/storage"
)

// Row is a shipped feature record.
type Row struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	CommitSHA   string `json:"commit_sha,omitempty"`
	Branch      string `json:"branch,omitempty"`
	CreatedAt   string `json:"created_at"`
}

// Add records a feature.
func Add(db *sql.DB, title, description, commitSHA, branch, modelID string) (string, error) {
	id, err := storage.NewID()
	if err != nil {
		return "", err
	}
	now := storage.NowUTC()
	_, err = db.Exec(`
		INSERT INTO features(id, title, description, commit_sha, branch, created_at, model_id)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, title, nullStr(description), nullStr(commitSHA), nullStr(branch), now, modelID,
	)
	return id, err
}

// List returns recent features. limit 0 means no cap.
func List(db *sql.DB, limit int) ([]Row, error) {
	if limit < 0 {
		limit = 20
	}
	q := `SELECT id, title, COALESCE(description,''), COALESCE(commit_sha,''), COALESCE(branch,''), created_at
		FROM features ORDER BY created_at DESC`
	q, args := storage.AppendLimit(q, nil, limit)
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Row
	for rows.Next() {
		var r Row
		if err := rows.Scan(&r.ID, &r.Title, &r.Description, &r.CommitSHA, &r.Branch, &r.CreatedAt); err != nil {
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
