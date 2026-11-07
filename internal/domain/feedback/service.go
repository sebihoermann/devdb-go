package feedback

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/sebihoermann/devdb-go/internal/storage"
)

// Row is a feedback ledger entry.
type Row struct {
	ID          string `json:"id"`
	Role        string `json:"role"`
	Category    string `json:"category,omitempty"`
	Severity    string `json:"severity,omitempty"`
	Note        string `json:"note"`
	Context     string `json:"context,omitempty"`
	Status      string `json:"status"`
	ProposedFix string `json:"proposed_fix,omitempty"`
	CreatedAt   string `json:"created_at"`
}

// AddInput captures a new feedback row.
type AddInput struct {
	Role     string
	Category string
	Severity string
	Note     string
	Context  string
	ModelID  string
}

// Add inserts feedback and returns the new id.
func Add(db *sql.DB, in AddInput) (string, error) {
	id, err := storage.NewID()
	if err != nil {
		return "", err
	}
	now := storage.NowUTC()
	_, err = db.Exec(`
		INSERT INTO feedback(id, role, category, severity, note, context, status, created_at, model_id)
		VALUES (?, ?, ?, ?, ?, ?, 'open', ?, ?)`,
		id, in.Role, nullIfEmpty(in.Category), normalizeSeverity(in.Severity),
		in.Note, nullIfEmpty(in.Context), now, in.ModelID,
	)
	return id, err
}

// List returns open or all feedback rows. limit 0 means no cap.
// The "all" sentinel means "no status filter" — same contract as
// goals.List, tasks.List and reminders.List. Passing it through literally
// would filter for status='all' and match nothing.
func List(db *sql.DB, status string, limit int) ([]Row, error) {
	if limit < 0 {
		limit = 20
	}
	q := `SELECT id, role, category, severity, note, context, status, COALESCE(proposed_fix,''), created_at
	      FROM feedback`
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
		var cat, sev, ctx, fix sql.NullString
		if err := rows.Scan(&r.ID, &r.Role, &cat, &sev, &r.Note, &ctx, &r.Status, &fix, &r.CreatedAt); err != nil {
			return nil, err
		}
		r.Category = cat.String
		r.Severity = sev.String
		r.Context = ctx.String
		r.ProposedFix = fix.String
		out = append(out, r)
	}
	return out, rows.Err()
}

func nullIfEmpty(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

func normalizeSeverity(s string) any {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return nil
	}
	if s == "medium" {
		return "med"
	}
	return s
}

// Annotate appends a timestamped note to feedback context.
func Annotate(db *sql.DB, idPrefix, note, modelID string) (string, error) {
	ids, err := allIDs(db)
	if err != nil {
		return "", err
	}
	id, err := storage.ResolveID(idPrefix, ids)
	if err != nil {
		return "", err
	}
	stamp := "[annotate " + storage.NowUTC() + "] " + note
	_, err = db.Exec(`
		UPDATE feedback SET context = CASE
			WHEN context IS NULL OR context = '' THEN ?
			ELSE context || char(10) || ?
		END WHERE id=?`, stamp, stamp, id)
	return id, err
}

// Close marks feedback resolved.
func Close(db *sql.DB, idPrefix, proposedFix, modelID string) (string, error) {
	ids, err := allIDs(db)
	if err != nil {
		return "", err
	}
	id, err := storage.ResolveID(idPrefix, ids)
	if err != nil {
		return "", err
	}
	_, err = db.Exec(`UPDATE feedback SET status='closed', proposed_fix=? WHERE id=?`, nullIfEmpty(proposedFix), id)
	return id, err
}

// SetSeverity re-seeds a feedback row's severity (e.g. high -> info when a
// row turns out to be an evidence/state record rather than actionable debt —
// the severity contract in skills/devdb/SKILL.md). The old->new change plus
// the optional reason are appended to context so the reclassification is
// auditable from the row itself. Returns the resolved id and the previous
// severity.
func SetSeverity(db *sql.DB, idPrefix, severity, reason, modelID string) (string, string, error) {
	ids, err := allIDs(db)
	if err != nil {
		return "", "", err
	}
	id, err := storage.ResolveID(idPrefix, ids)
	if err != nil {
		return "", "", err
	}
	var old string
	if err := db.QueryRow(`SELECT COALESCE(severity,'') FROM feedback WHERE id=?`, id).Scan(&old); err != nil {
		return "", "", err
	}
	normalized := normalizeSeverity(severity)
	newSev, _ := normalized.(string)
	if newSev == "" {
		return "", "", fmt.Errorf("feedback: empty severity")
	}
	if _, err := db.Exec(`UPDATE feedback SET severity=? WHERE id=?`, newSev, id); err != nil {
		return "", "", err
	}
	stamp := "[severity " + storage.NowUTC() + "] " + old + " -> " + newSev
	if strings.TrimSpace(reason) != "" {
		stamp += ": " + strings.TrimSpace(reason)
	}
	if _, err := db.Exec(`
		UPDATE feedback SET context = CASE
			WHEN context IS NULL OR context = '' THEN ?
			ELSE context || char(10) || ?
		END WHERE id=?`, stamp, stamp, id); err != nil {
		return "", "", err
	}
	return id, old, nil
}

func allIDs(db *sql.DB) ([]string, error) {
	rows, err := db.Query(`SELECT id FROM feedback`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// Show returns one feedback row.
func Show(db *sql.DB, idPrefix string) (Row, error) {
	ids, err := allIDs(db)
	if err != nil {
		return Row{}, err
	}
	id, err := storage.ResolveID(idPrefix, ids)
	if err != nil {
		return Row{}, err
	}
	var r Row
	var cat, sev, ctx, fix sql.NullString
	err = db.QueryRow(`
		SELECT id, role, category, severity, note, context, status, COALESCE(proposed_fix,''), created_at
		FROM feedback WHERE id=?`, id,
	).Scan(&r.ID, &r.Role, &cat, &sev, &r.Note, &ctx, &r.Status, &fix, &r.CreatedAt)
	if err != nil {
		return Row{}, fmt.Errorf("feedback not found: %w", err)
	}
	r.Category = cat.String
	r.Severity = sev.String
	r.Context = ctx.String
	r.ProposedFix = fix.String
	return r, nil
}
