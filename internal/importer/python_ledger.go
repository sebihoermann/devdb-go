package importer

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sebihoermann/devdb-go/internal/migrate"
	"github.com/sebihoermann/devdb-go/internal/storage"
)

// ErrPythonBakAlreadyMigrated is returned by ApplyInPlace when a sibling
// .python-bak file already holds Go-schema data — a signal that this DB has
// been migrated before and the rollback artifact has been overwritten. Pass
// force=true (CLI: --force) to ignore the check.
var ErrPythonBakAlreadyMigrated = errors.New("sibling .python-bak has Go schema — already migrated; pass --force to ignore")

var renameFile = os.Rename

// PythonLedgerInfo describes a legacy database.
type PythonLedgerInfo struct {
	Path    string `json:"path"`
	Version int    `json:"version"`
	Tables  int    `json:"tables"`
}

// ImportResult reports rows copied per table during a Python→Go import.
type ImportResult struct {
	SourcePath string         `json:"source_path"`
	DestPath   string         `json:"dest_path"`
	Tables     map[string]int `json:"tables"`
	Skipped    []string       `json:"skipped_python_only"`
	Archived   []ArchiveSpec  `json:"archived,omitempty"`
}

// pythonOnlyTables are legacy tables with no Go-native destination.
var pythonOnlyTables = []string{
	"code_tightening",
	"loc_snapshots",
	"improvement_suggestions",
	"agent_documents",
	"anchors",
	"entity_links",
	"project_links",
}

// InspectPythonDB opens a Python-created database read-only and reports metadata.
func InspectPythonDB(path string) (PythonLedgerInfo, error) {
	db, err := storage.Open(path)
	if err != nil {
		return PythonLedgerInfo{}, err
	}
	defer db.Close()

	kind, version, err := storage.DetectSchema(db)
	if err != nil {
		return PythonLedgerInfo{}, err
	}
	if kind != storage.SchemaPython {
		return PythonLedgerInfo{}, fmt.Errorf("%s is not a legacy Python devdb (found %s)", path, kind)
	}
	var tables int
	_ = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table'`).Scan(&tables)
	return PythonLedgerInfo{Path: path, Version: version, Tables: tables}, nil
}

// ImportPythonDB copies data from a legacy database into a fresh Go-native database.
func ImportPythonDB(srcPath, dstPath string, replace bool) (ImportResult, error) {
	srcAbs, err := filepath.Abs(srcPath)
	if err != nil {
		return ImportResult{}, err
	}
	dstAbs, err := filepath.Abs(dstPath)
	if err != nil {
		return ImportResult{}, err
	}
	if srcAbs == dstAbs {
		return ImportResult{}, fmt.Errorf("source and destination must differ — use --apply for in-place migration")
	}

	if _, err := InspectPythonDB(srcPath); err != nil {
		return ImportResult{}, err
	}

	if err := os.MkdirAll(filepath.Dir(dstAbs), 0o755); err != nil {
		return ImportResult{}, err
	}

	if fileExists(dstAbs) {
		kind, _, err := detectFileSchema(dstAbs)
		if err != nil {
			return ImportResult{}, err
		}
		switch kind {
		case storage.SchemaPython:
			return ImportResult{}, fmt.Errorf("destination is a legacy Python database — choose an empty or Go-native path")
		case storage.SchemaGo:
			if !replace {
				rows, err := countGoRows(dstAbs)
				if err != nil {
					return ImportResult{}, err
				}
				if rows > 0 {
					return ImportResult{}, fmt.Errorf("destination already has data — pass --replace to overwrite")
				}
			}
		}
		if err := os.Remove(dstAbs); err != nil {
			return ImportResult{}, err
		}
	}

	dst, err := storage.Open(dstAbs)
	if err != nil {
		return ImportResult{}, err
	}
	defer dst.Close()

	if err := migrate.RunAll(dst); err != nil {
		return ImportResult{}, err
	}

	result := ImportResult{
		SourcePath: srcAbs,
		DestPath:   dstAbs,
		Tables:     map[string]int{},
		Skipped:    append([]string(nil), pythonOnlyTables...),
	}

	if err := copyLegacyData(dst, srcAbs, result.Tables); err != nil {
		return ImportResult{}, err
	}
	return result, nil
}

// ApplyInPlace migrates the project database from Python to Go schema atomically.
// When noArchive is false (default), populated python-only tables are JSONL-archived
// to <.devdb>/archive-python-only/ before the python DB is moved aside as
// development.db.python-bak. Pass noArchive=true to skip the archive step.
// When force is false (default), the function errors with ErrPythonBakAlreadyMigrated
// if a sibling .python-bak already holds Go-schema data — pass force=true to ignore.
func ApplyInPlace(dbPath string, noArchive, force bool) (ImportResult, error) {
	abs, err := filepath.Abs(dbPath)
	if err != nil {
		return ImportResult{}, err
	}
	if _, err := InspectPythonDB(abs); err != nil {
		return ImportResult{}, err
	}
	dir := filepath.Dir(abs)
	tmp := filepath.Join(dir, ".development.importing.db")
	backup := filepath.Join(dir, "development.db.python-bak")
	bakPath := abs + ".python-bak"
	if !force {
		if _, err := os.Stat(bakPath); err == nil {
			if kind, _, err := detectFileSchema(bakPath); err == nil && kind == storage.SchemaGo {
				return ImportResult{}, ErrPythonBakAlreadyMigrated
			}
		}
	}
	_ = os.Remove(tmp)

	result, err := ImportPythonDB(abs, tmp, true)
	if err != nil {
		_ = os.Remove(tmp)
		return ImportResult{}, err
	}
	if !noArchive {
		srcDB, err := storage.Open(abs)
		if err != nil {
			return ImportResult{}, fmt.Errorf("open source for archive: %w", err)
		}
		archiveDir := filepath.Join(dir, "archive-python-only")
		archived, err := ArchivePythonOnly(srcDB, archiveDir)
		srcDB.Close()
		if err != nil {
			_ = os.Remove(tmp)
			return ImportResult{}, fmt.Errorf("archive python-only: %w", err)
		}
		result.Archived = archived
	}
	_ = os.Remove(backup)
	if err := renameFile(abs, backup); err != nil {
		_ = os.Remove(tmp)
		return ImportResult{}, fmt.Errorf("backup %s: %w", backup, err)
	}
	if err := renameFile(tmp, abs); err != nil {
		restoreErr := renameFile(backup, abs)
		_ = os.Remove(tmp)
		if restoreErr != nil {
			return ImportResult{}, fmt.Errorf("replace database: %w; restore backup: %v", err, restoreErr)
		}
		return ImportResult{}, fmt.Errorf("replace database: %w", err)
	}
	result.DestPath = abs
	return result, nil
}

func copyLegacyData(dst *sql.DB, srcAbs string, counts map[string]int) error {
	attachSQL := fmt.Sprintf("ATTACH DATABASE %q AS legacy", filepath.ToSlash(srcAbs))
	if _, err := dst.Exec(attachSQL); err != nil {
		return fmt.Errorf("attach legacy db: %w", err)
	}
	defer func() { _, _ = dst.Exec("DETACH DATABASE legacy") }()

	if _, err := dst.Exec("PRAGMA foreign_keys = OFF"); err != nil {
		return err
	}
	defer func() {
		_, _ = dst.Exec("PRAGMA foreign_keys = ON")
	}()
	// Run every legacy-table insert inside one transaction so a mid-sequence
	// failure rolls back the partial import. Detach and the deferred FK restore
	// stay outside the rollback boundary because they are infrastructure, not
	// imported data.
	if err := storage.WithTx(dst, func(tx *sql.Tx) error {
		for _, spec := range copySpecs() {
			res, err := tx.Exec(spec.sql)
			if err != nil {
				if strings.Contains(err.Error(), "no such table") {
					continue
				}
				return fmt.Errorf("copy %s: %w", spec.table, err)
			}
			n, _ := res.RowsAffected()
			counts[spec.table] = int(n)
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}

type copySpec struct {
	table string
	sql   string
}

func copySpecs() []copySpec {
	return []copySpec{
		{"goals", `INSERT OR REPLACE INTO goals (id, kind, title, body, status, created_at, model_id)
			SELECT id, kind, title, body,
				CASE WHEN status IN ('active','done','wontfix') THEN status
				     WHEN status = 'inactive' THEN 'wontfix'
				     WHEN status IS NULL OR status = '' THEN 'active'
				     ELSE 'active' END,
				created_at, model_id FROM legacy.goals`},
		{"feedback", `INSERT OR REPLACE INTO feedback (id, role, category, severity, note, context, status, proposed_fix, created_at, model_id)
			SELECT id, role, category, severity, note, context,
				CASE WHEN status = 'open' THEN 'open'
				     WHEN status IN ('deferred','wontfix','resolved','closed') THEN 'closed'
				     WHEN status IS NULL OR status = '' THEN 'open'
				     ELSE 'closed' END,
				proposed_fix, created_at, model_id FROM legacy.feedback`},
		{"features", `INSERT OR REPLACE INTO features SELECT id, title, description, commit_sha, branch, created_at, model_id FROM legacy.features`},
		{"plans", `INSERT OR REPLACE INTO plans SELECT id, slug, title, body, status, created_at, model_id FROM legacy.plans`},
		{"milestones", `INSERT OR REPLACE INTO milestones (id, plan_id, number, title, body, status, created_at, model_id)
			SELECT id, plan_id, number, title, body,
				CASE WHEN status IN ('planned','in_progress','done','wontfix') THEN status
				     WHEN status IS NULL OR status = '' THEN 'planned'
				     ELSE 'planned' END,
				created_at, model_id FROM legacy.milestones`},
		{"plan_items", `INSERT OR REPLACE INTO plan_items (id, plan_id, milestone_id, item_number, phase, step, title, body, source_doc, status, approval_status, created_at, model_id)
			SELECT id, plan_id, milestone_id, item_number, phase, step, title, body, source_doc,
				CASE WHEN status IN ('planned','in_progress','done','wontfix') THEN status
				     WHEN status IS NULL OR status = '' THEN 'planned'
				     ELSE 'planned' END,
				approval_status, created_at, model_id FROM legacy.plan_items`},
		{"plan_item_acceptance", `INSERT OR REPLACE INTO plan_item_acceptance (id, plan_item_id, ordinal, criterion, status, evidence, created_at, updated_at, model_id)
			SELECT id, plan_item_id, ordinal, criterion,
				CASE WHEN status IN ('open','met') THEN status
				     WHEN status IS NULL OR status = '' THEN 'open'
				     ELSE 'open' END,
				evidence, created_at, updated_at, model_id FROM legacy.plan_item_acceptance`},
		{"plan_item_files", `INSERT OR REPLACE INTO plan_item_files SELECT id, plan_item_id, path, role, created_at, model_id FROM legacy.plan_item_files`},
		{"status_log", `INSERT OR REPLACE INTO status_log SELECT id, plan_item_id, status, note, created_at, model_id FROM legacy.status_log`},
		{"tasks", `INSERT OR REPLACE INTO tasks (id, title, body, status, priority, due_at, approval_status, created_at, model_id)
			SELECT id, title, body,
				CASE WHEN status IN ('open','done','wontfix') THEN status
				     WHEN status IS NULL OR status = '' THEN 'open'
				     ELSE 'open' END,
				priority, due_at, approval_status, created_at, model_id FROM legacy.tasks`},
		{"reminders", `INSERT OR REPLACE INTO reminders (id, title, body, due_at, file_path, plan_item_id, status, snooze_until, created_at, model_id)
			SELECT id, title, body, due_at, file_path, plan_item_id,
				CASE WHEN status IN ('open','dismissed') THEN status
				     WHEN status IS NULL OR status = '' THEN 'open'
				     ELSE 'open' END,
				snooze_until, created_at, model_id FROM legacy.reminders`},
		{"approval_log", `INSERT OR REPLACE INTO approval_log SELECT id, entity_table, entity_id, action, note, created_at, model_id FROM legacy.approval_log`},
		{"scan_runs", `INSERT OR REPLACE INTO scan_runs SELECT id, started_at, finished_at, git_sha, files_seen, files_added, files_changed, files_removed, model_id FROM legacy.scan_runs`},
		{"repo_files", `INSERT OR REPLACE INTO repo_files SELECT path, language, kind, lines, content_hash, size_bytes, last_seen_at, last_scan_run_id FROM legacy.repo_files`},
		{"file_change_events", `INSERT OR REPLACE INTO file_change_events SELECT id, scan_run_id, path, change_kind, old_hash, new_hash, created_at, model_id FROM legacy.file_change_events`},
		{"architecture_notes", `INSERT OR REPLACE INTO architecture_notes SELECT id, topic, body, source_paths, source_hashes, confidence, status, last_verified_at, created_at, updated_at, model_id FROM legacy.architecture_notes`},
		{"review_runs", `INSERT OR REPLACE INTO review_runs SELECT id, scope_paths, tier, started_at, finished_at, git_sha, files_total, files_reviewed, summary, model_id FROM legacy.review_runs`},
		{"review_findings", `INSERT OR REPLACE INTO review_findings SELECT id, run_id, file_path, line_start, line_end, principle, title, recommendation, severity, confidence, effort, status, resolved_in_commit, source_hash, created_at, model_id FROM legacy.review_findings`},
		{"verification_runs", `INSERT OR REPLACE INTO verification_runs (id, command, scope, status, git_sha, exit_code, output, notes, started_at, finished_at, dismissed_at, dismissed_reason, created_at, model_id)
			SELECT id, command, scope, status, git_sha, exit_code, output, notes, started_at, finished_at, dismissed_at, dismissed_reason, created_at, model_id FROM legacy.verification_runs`},
		{"verification_inputs", `INSERT OR REPLACE INTO verification_inputs SELECT id, run_id, file_path, role, content_hash, created_at, model_id FROM legacy.verification_inputs`},
		{"verification_failures", `INSERT OR REPLACE INTO verification_failures SELECT id, run_id, test_id, file_path, headline, message_excerpt, failure_kind, created_at, model_id FROM legacy.verification_failures`},
		{"missed_cli_calls", `INSERT OR REPLACE INTO missed_cli_calls SELECT id, raw_argv, normalized_command, failure_kind, error_message, suggested_command, exit_code, cwd, repo_root, model_id, created_at FROM legacy.missed_cli_calls`},
		{"archive_entries", `INSERT OR REPLACE INTO archive_entries SELECT id, source_table, source_id, payload_json, archived_at, archive_reason FROM legacy.archive_entries`},
		{"sync_state", `INSERT OR REPLACE INTO sync_state SELECT key, value, updated_at FROM legacy.sync_state`},
		{"commit_archeology", `INSERT OR REPLACE INTO commit_archeology
			SELECT id, branch, sha, author, committed_at, subject, body, intent_tag, created_at, model_id
			FROM legacy.commit_archeology`},
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func detectFileSchema(path string) (storage.SchemaKind, int, error) {
	db, err := storage.Open(path)
	if err != nil {
		return storage.SchemaUnknown, 0, err
	}
	defer db.Close()
	return storage.DetectSchema(db)
}

func countGoRows(path string) (int, error) {
	db, err := storage.Open(path)
	if err != nil {
		return 0, err
	}
	defer db.Close()
	var n int
	err = db.QueryRow(`
		SELECT
			(SELECT COUNT(*) FROM feedback) +
			(SELECT COUNT(*) FROM plan_items) +
			(SELECT COUNT(*) FROM goals)
	`).Scan(&n)
	return n, err
}

// ParityCounts holds row counts for parity verification.
type ParityCounts struct {
	Feedback   int `json:"feedback"`
	PlanItems  int `json:"plan_items"`
	Goals      int `json:"goals"`
	Plans      int `json:"plans"`
	RepoFiles  int `json:"repo_files"`
	VerifyRuns int `json:"verification_runs"`
	Archive    int `json:"archive_entries"`
}

// CountParity reads core table counts from any devdb database.
func CountParity(db *sql.DB) (ParityCounts, error) {
	var c ParityCounts
	queries := []struct {
		sql string
		dst *int
	}{
		{`SELECT COUNT(*) FROM feedback`, &c.Feedback},
		{`SELECT COUNT(*) FROM plan_items`, &c.PlanItems},
		{`SELECT COUNT(*) FROM goals`, &c.Goals},
		{`SELECT COUNT(*) FROM plans`, &c.Plans},
		{`SELECT COUNT(*) FROM repo_files`, &c.RepoFiles},
		{`SELECT COUNT(*) FROM verification_runs`, &c.VerifyRuns},
		{`SELECT COUNT(*) FROM archive_entries`, &c.Archive},
	}
	for _, q := range queries {
		if err := db.QueryRow(q.sql).Scan(q.dst); err != nil {
			if strings.Contains(err.Error(), "no such table") {
				continue
			}
			return ParityCounts{}, err
		}
	}
	return c, nil
}
