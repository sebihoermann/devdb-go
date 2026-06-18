// Package analytics records and reports failed CLI invocations for agent UX improvement.
package analytics

import (
	"database/sql"
	"strings"
	"time"

	"github.com/sebihoermann/devdb-go/internal/storage"
)

// RecordMissedCall logs a failed devdb invocation when the database is available.
func RecordMissedCall(db *sql.DB, rawArgv []string, kind, message, suggestion string, exitCode int, cwd, repoRoot, modelID string) {
	if db == nil {
		return
	}
	id, err := storage.NewID()
	if err != nil {
		return
	}
	normalized := ""
	if len(rawArgv) > 0 {
		normalized = strings.Join(rawArgv, " ")
	}
	_, _ = db.Exec(`
		INSERT INTO missed_cli_calls(
			id, raw_argv, normalized_command, failure_kind, error_message,
			suggested_command, exit_code, cwd, repo_root, model_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, strings.Join(rawArgv, " "), normalized, kind, message,
		nullIfEmpty(suggestion), exitCode, cwd, repoRoot, modelID, storage.NowUTC(),
	)
}

func nullIfEmpty(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

// MissedRow is one failed CLI invocation.
type MissedRow struct {
	ID                string `json:"id"`
	CreatedAt         string `json:"created_at"`
	FailureKind       string `json:"failure_kind"`
	RawArgv           string `json:"raw_argv"`
	ErrorMessage      string `json:"error_message"`
	SuggestedCommand  string `json:"suggested_command,omitempty"`
}

// ListMissedCalls returns recent missed CLI calls.
func ListMissedCalls(db *sql.DB, since string, limit int) ([]MissedRow, error) {
	if since == "" {
		since = time.Now().UTC().AddDate(0, 0, -7).Format(time.RFC3339Nano)
	}
	q := `
		SELECT id, created_at, failure_kind, raw_argv, error_message, COALESCE(suggested_command,'')
		FROM missed_cli_calls WHERE created_at >= ?
		ORDER BY created_at DESC`
	args := []any{since}
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]MissedRow, 0)
	for rows.Next() {
		var r MissedRow
		if err := rows.Scan(&r.ID, &r.CreatedAt, &r.FailureKind, &r.RawArgv, &r.ErrorMessage, &r.SuggestedCommand); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// KindCount groups failures by kind.
type KindCount struct {
	Kind  string `json:"kind"`
	Count int    `json:"count"`
}

// CommandCount groups failures by normalized command.
type CommandCount struct {
	Command string `json:"command"`
	Count   int    `json:"count"`
}

// Summary is aggregated missed-call telemetry.
type Summary struct {
	Total           int            `json:"total"`
	WindowDays      int            `json:"window_days"`
	TopFailureKinds []KindCount    `json:"top_failure_kinds"`
	TopCommands     []CommandCount `json:"top_commands"`
}

// MissedSummary aggregates missed-call patterns since a timestamp.
func MissedSummary(db *sql.DB, since string, windowDays int) (Summary, error) {
	if windowDays <= 0 {
		windowDays = 7
	}
	if since == "" {
		since = time.Now().UTC().AddDate(0, 0, -windowDays).Format(time.RFC3339Nano)
	}
	var s Summary
	s.WindowDays = windowDays
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM missed_cli_calls WHERE created_at >= ?`, since,
	).Scan(&s.Total); err != nil {
		return s, err
	}
	kindRows, err := db.Query(`
		SELECT failure_kind, COUNT(*) FROM missed_cli_calls WHERE created_at >= ?
		GROUP BY failure_kind ORDER BY COUNT(*) DESC LIMIT 5`, since)
	if err != nil {
		return s, err
	}
	defer kindRows.Close()
	for kindRows.Next() {
		var k KindCount
		if err := kindRows.Scan(&k.Kind, &k.Count); err != nil {
			return s, err
		}
		s.TopFailureKinds = append(s.TopFailureKinds, k)
	}
	cmdRows, err := db.Query(`
		SELECT normalized_command, COUNT(*) FROM missed_cli_calls
		WHERE created_at >= ? AND normalized_command IS NOT NULL
		GROUP BY normalized_command ORDER BY COUNT(*) DESC LIMIT 5`, since)
	if err != nil {
		return s, err
	}
	defer cmdRows.Close()
	for cmdRows.Next() {
		var c CommandCount
		if err := cmdRows.Scan(&c.Command, &c.Count); err != nil {
			return s, err
		}
		s.TopCommands = append(s.TopCommands, c)
	}
	return s, cmdRows.Err()
}

// HygieneReport is per-repo CLI hygiene diagnostics.
type HygieneReport struct {
	MissedCalls7d     int            `json:"missed_cli_calls_7d"`
	TopFailureKinds   []KindCount    `json:"top_failure_kinds"`
	TopCommands       []CommandCount `json:"top_commands"`
	ActiveArchNotes   int            `json:"active_arch_notes"`
	Recommendations   []string       `json:"recommendations"`
}

// Hygiene builds doctor hygiene diagnostics.
func Hygiene(db *sql.DB) (HygieneReport, error) {
	since := time.Now().UTC().AddDate(0, 0, -7).Format(time.RFC3339Nano)
	sum, err := MissedSummary(db, since, 7)
	if err != nil {
		return HygieneReport{}, err
	}
	var activeArch int
	_ = db.QueryRow(`SELECT COUNT(*) FROM architecture_notes WHERE status='active'`).Scan(&activeArch)
	rep := HygieneReport{
		MissedCalls7d:   sum.Total,
		TopFailureKinds: sum.TopFailureKinds,
		TopCommands:     sum.TopCommands,
		ActiveArchNotes: activeArch,
	}
	if sum.Total > 20 {
		rep.Recommendations = append(rep.Recommendations,
			"run devdb analytics summary and fix top unknown_command typos")
	}
	if activeArch > 5 {
		rep.Recommendations = append(rep.Recommendations,
			"run devdb arch verify (when wired) or devdb archive gc --dry-run")
	}
	return rep, nil
}
