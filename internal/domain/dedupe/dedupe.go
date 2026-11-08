// Package dedupe finds existing ledger rows that duplicate a proposed add.
//
// The check is intentionally text-based and normalization-tolerant: a note
// re-entered with different casing or spacing is still the same note. It is
// advisory infrastructure for write verbs, not a uniqueness constraint — the
// schema stays append-only, and callers decide whether a match blocks them.
package dedupe

import (
	"database/sql"
	"fmt"
	"strings"
)

// Match identifies an existing row whose text duplicates a proposed add.
type Match struct {
	ID   string
	Text string
}

// Filter is an equality constraint on a column. COALESCE makes NULL and the
// empty string compare equal, which is what callers mean by "no value".
type Filter struct {
	Column string
	Value  string
}

// Query describes a duplicate lookup over one ledger table. Table and
// TextColumn are internal constants supplied by callers, never user input.
type Query struct {
	Table      string
	TextColumn string
	Text       string
	Filters    []Filter
}

// Normalize lowercases and collapses internal whitespace so "Fix  the bug"
// and "fix the bug" compare equal.
func Normalize(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

// Find returns the first row in q.Table whose q.TextColumn normalizes to
// q.Text while satisfying every q.Filter. It returns nil when nothing matches.
func Find(db *sql.DB, q Query) (*Match, error) {
	target := Normalize(q.Text)
	if target == "" {
		return nil, nil
	}
	query := fmt.Sprintf("SELECT id, %s FROM %s", q.TextColumn, q.Table)
	var where []string
	var args []any
	for _, f := range q.Filters {
		where = append(where, fmt.Sprintf("COALESCE(%s,'') = ?", f.Column))
		args = append(args, f.Value)
	}
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, text string
		if err := rows.Scan(&id, &text); err != nil {
			return nil, err
		}
		if Normalize(text) == target {
			return &Match{ID: id, Text: text}, nil
		}
	}
	return nil, rows.Err()
}
