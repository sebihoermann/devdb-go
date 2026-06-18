package archive

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sebihoermann/devdb-go/internal/domain/architecture"
	"github.com/sebihoermann/devdb-go/internal/storage"
)

// RunOptions controls archive sweep behavior.
type RunOptions struct {
	SessionHours  int
	KeepSnapshots int
	Table         string
	DryRun        bool
	Yes           bool
	Vacuum        bool
}

// RunResult summarizes an archive sweep.
type RunResult struct {
	WouldArchiveTotal int            `json:"would_archive_total_rows,omitempty"`
	ArchivedTotal     int            `json:"archived_total,omitempty"`
	ByTable           map[string]int `json:"by_table"`
	SessionHours      int            `json:"session_hours,omitempty"`
	KeepSnapshots     int            `json:"keep_snapshots,omitempty"`
	DryRun            bool           `json:"dry_run,omitempty"`
}

type archiveStep struct {
	table  string
	where  string
	params []any
	reason string
	bundle string // "by_snapshot_at" or ""
}

// Run moves historical closed/resolved rows into archive_entries.
func Run(db *sql.DB, opt RunOptions) (RunResult, error) {
	if opt.SessionHours <= 0 {
		opt.SessionHours = 24
	}
	if opt.KeepSnapshots <= 0 {
		opt.KeepSnapshots = 3
	}
	cutoff := time.Now().UTC().Add(-time.Duration(opt.SessionHours) * time.Hour).Format(time.RFC3339Nano)

	plan := []archiveStep{
		{"feedback", "status='closed' AND created_at < ?", []any{cutoff}, "closed pre-session", ""},
		{"review_findings", "status IN ('resolved','wontfix','accepted','duplicate')", nil, "resolved/wontfix/accepted/duplicate", ""},
		{"review_runs", "finished_at IS NOT NULL AND id NOT IN (SELECT DISTINCT run_id FROM review_findings WHERE status='open')", nil, "finished run, no open findings remain", ""},
		{"features", "1=1", nil, "historical feature log", ""},
		{"status_log", "created_at < ?", []any{cutoff}, "pre-session log", ""},
		{"plan_items", "status IN ('done','wontfix') AND created_at < ?", []any{cutoff}, "completed plan, pre-session", ""},
		{"reminders", "status='dismissed' AND created_at < ?", []any{cutoff}, "dismissed pre-session", ""},
		{"tasks", "status IN ('done','wontfix') AND created_at < ?", []any{cutoff}, "closed pre-session", ""},
	}

	if opt.Table != "" {
		var filtered []archiveStep
		for _, s := range plan {
			if s.table == opt.Table {
				filtered = append(filtered, s)
			}
		}
		if len(filtered) == 0 {
			return RunResult{}, fmt.Errorf("unknown table for archive: %q", opt.Table)
		}
		plan = filtered
	}

	counts := make(map[string]int)
	for _, step := range plan {
		exists, err := storage.TableExists(db, step.table)
		if err != nil {
			return RunResult{}, err
		}
		if !exists {
			counts[step.table] = 0
			continue
		}
		n, err := countRows(db, step.table, step.where, step.params, step.bundle, opt.KeepSnapshots)
		if err != nil {
			return RunResult{}, err
		}
		counts[step.table] = n
	}

	total := 0
	for _, n := range counts {
		total += n
	}

	if opt.DryRun {
		return RunResult{
			WouldArchiveTotal: total,
			ByTable:           counts,
			SessionHours:      opt.SessionHours,
			KeepSnapshots:     opt.KeepSnapshots,
			DryRun:            true,
		}, nil
	}
	if total == 0 {
		return RunResult{ArchivedTotal: 0, ByTable: counts}, nil
	}

	archivedAt := storage.NowUTC()
	archived := make(map[string]int)

	for _, step := range plan {
		exists, _ := storage.TableExists(db, step.table)
		if !exists || counts[step.table] == 0 {
			continue
		}
		n, err := archiveTable(db, step, archivedAt, opt.KeepSnapshots)
		if err != nil {
			return RunResult{}, err
		}
		archived[step.table] = n
	}

	if opt.Table == "" || opt.Table == "plan_items" {
		for _, child := range []string{"plan_item_acceptance", "plan_item_files"} {
			n, err := archiveOrphans(db, child, archivedAt)
			if err != nil {
				return RunResult{}, err
			}
			if n > 0 {
				archived[child] = n
			}
		}
	}

	if opt.Vacuum {
		if _, err := db.Exec("VACUUM"); err != nil {
			return RunResult{}, err
		}
	}

	sum := 0
	for _, n := range archived {
		sum += n
	}
	return RunResult{ArchivedTotal: sum, ByTable: archived}, nil
}

func countRows(db *sql.DB, table, where string, params []any, bundle string, keepSnaps int) (int, error) {
	if bundle == "by_snapshot_at" {
		return countLocSnapshots(db, keepSnaps)
	}
	q := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s", table, where)
	var n int
	err := db.QueryRow(q, params...).Scan(&n)
	return n, err
}

func countLocSnapshots(db *sql.DB, keep int) (int, error) {
	exists, err := storage.TableExists(db, "loc_snapshots")
	if err != nil || !exists {
		return 0, err
	}
	rows, err := db.Query(
		`SELECT DISTINCT snapshot_at FROM loc_snapshots ORDER BY snapshot_at DESC LIMIT ?`, keep,
	)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var kept []any
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return 0, err
		}
		kept = append(kept, s)
	}
	if len(kept) == 0 {
		return 0, rows.Err()
	}
	ph := strings.Repeat("?,", len(kept))
	ph = ph[:len(ph)-1]
	var n int
	err = db.QueryRow(
		fmt.Sprintf(`SELECT COUNT(*) FROM loc_snapshots WHERE snapshot_at NOT IN (%s)`, ph),
		kept...,
	).Scan(&n)
	return n, err
}

func archiveTable(db *sql.DB, step archiveStep, archivedAt string, keepSnaps int) (int, error) {
	if step.bundle == "by_snapshot_at" {
		return archiveLocSnapshots(db, archivedAt, step.reason, keepSnaps)
	}

	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	q := fmt.Sprintf("SELECT * FROM %s WHERE %s", step.table, step.where)
	rows, err := tx.Query(q, step.params...)
	if err != nil {
		return 0, err
	}
	cols, err := rows.Columns()
	if err != nil {
		rows.Close()
		return 0, err
	}

	type rowData struct {
		payload map[string]any
		id      string
	}
	var collected []rowData
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			rows.Close()
			return 0, err
		}
		payload := make(map[string]any, len(cols))
		var id string
		for i, c := range cols {
			v := vals[i]
			if b, ok := v.([]byte); ok {
				v = string(b)
			}
			payload[c] = v
			if c == "id" && v != nil {
				id = fmt.Sprint(v)
			}
		}
		collected = append(collected, rowData{payload: payload, id: id})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	var ids []string
	for _, r := range collected {
		payloadJSON, err := json.Marshal(r.payload)
		if err != nil {
			return 0, err
		}
		archID, err := storage.NewID()
		if err != nil {
			return 0, err
		}
		if _, err := tx.Exec(`
			INSERT INTO archive_entries(id, source_table, source_id, payload_json, archived_at, archive_reason)
			VALUES (?, ?, ?, ?, ?, ?)`,
			archID, step.table, r.id, string(payloadJSON), archivedAt, step.reason,
		); err != nil {
			return 0, err
		}
		if r.id != "" {
			ids = append(ids, r.id)
		}
	}

	if step.table == "plan_items" && len(ids) > 0 {
		if err := archivePlanItemChildren(tx, ids, archivedAt); err != nil {
			return 0, err
		}
	}

	if len(ids) > 0 {
		ph := strings.Repeat("?,", len(ids))
		ph = ph[:len(ph)-1]
		args := make([]any, len(ids))
		for i, id := range ids {
			args[i] = id
		}
		if _, err := tx.Exec(
			fmt.Sprintf("DELETE FROM %s WHERE id IN (%s)", step.table, ph), args...,
		); err != nil {
			return 0, fmt.Errorf("archive: failed to delete from %s: %w", step.table, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(collected), nil
}

func archivePlanItemChildren(tx *sql.Tx, parentIDs []string, archivedAt string) error {
	ph := strings.Repeat("?,", len(parentIDs))
	ph = ph[:len(ph)-1]
	args := make([]any, len(parentIDs))
	for i, id := range parentIDs {
		args[i] = id
	}
	for _, childTable := range []string{"plan_item_acceptance", "plan_item_files", "status_log"} {
		q := fmt.Sprintf("SELECT * FROM %s WHERE plan_item_id IN (%s)", childTable, ph)
		rows, err := tx.Query(q, args...)
		if err != nil {
			return err
		}
		cols, _ := rows.Columns()
		for rows.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				rows.Close()
				return err
			}
			payload := make(map[string]any, len(cols))
			var srcID string
			for i, c := range cols {
				v := vals[i]
				if b, ok := v.([]byte); ok {
					v = string(b)
				}
				payload[c] = v
				if c == "id" && v != nil {
					srcID = fmt.Sprint(v)
				}
			}
			payloadJSON, _ := json.Marshal(payload)
			archID, _ := storage.NewID()
			if _, err := tx.Exec(`
				INSERT INTO archive_entries(id, source_table, source_id, payload_json, archived_at, archive_reason)
				VALUES (?, ?, ?, ?, ?, ?)`,
				archID, childTable, srcID, string(payloadJSON), archivedAt, "child of archived plan_item",
			); err != nil {
				rows.Close()
				return err
			}
			if srcID != "" {
				if _, err := tx.Exec(fmt.Sprintf("DELETE FROM %s WHERE id=?", childTable), srcID); err != nil {
					rows.Close()
					return err
				}
			}
		}
		rows.Close()
	}
	return nil
}

func archiveOrphans(db *sql.DB, table, archivedAt string) (int, error) {
	exists, err := storage.TableExists(db, table)
	if err != nil || !exists {
		return 0, err
	}
	q := fmt.Sprintf("SELECT * FROM %s WHERE plan_item_id NOT IN (SELECT id FROM plan_items)", table)
	rows, err := db.Query(q)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	count := 0
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return count, err
		}
		payload := make(map[string]any, len(cols))
		var srcID string
		for i, c := range cols {
			v := vals[i]
			if b, ok := v.([]byte); ok {
				v = string(b)
			}
			payload[c] = v
			if c == "id" && v != nil {
				srcID = fmt.Sprint(v)
			}
		}
		payloadJSON, _ := json.Marshal(payload)
		archID, _ := storage.NewID()
		if _, err := db.Exec(`
			INSERT INTO archive_entries(id, source_table, source_id, payload_json, archived_at, archive_reason)
			VALUES (?, ?, ?, ?, ?, ?)`,
			archID, table, srcID, string(payloadJSON), archivedAt, "orphaned (parent plan archived)",
		); err != nil {
			return count, err
		}
		if srcID != "" {
			if _, err := db.Exec(fmt.Sprintf("DELETE FROM %s WHERE id=?", table), srcID); err != nil {
				return count, err
			}
		}
		count++
	}
	return count, rows.Err()
}

func archiveLocSnapshots(db *sql.DB, archivedAt, reason string, keep int) (int, error) {
	exists, err := storage.TableExists(db, "loc_snapshots")
	if err != nil || !exists {
		return 0, err
	}
	rows, err := db.Query(
		`SELECT DISTINCT snapshot_at FROM loc_snapshots ORDER BY snapshot_at DESC LIMIT ?`, keep,
	)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var kept []any
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return 0, err
		}
		kept = append(kept, s)
	}
	if len(kept) == 0 {
		return 0, rows.Err()
	}
	ph := strings.Repeat("?,", len(kept))
	ph = ph[:len(ph)-1]
	snapRows, err := db.Query(
		fmt.Sprintf(`SELECT DISTINCT snapshot_at FROM loc_snapshots WHERE snapshot_at NOT IN (%s)`, ph),
		kept...,
	)
	if err != nil {
		return 0, err
	}
	defer snapRows.Close()
	bundled := 0
	for snapRows.Next() {
		var snap string
		if err := snapRows.Scan(&snap); err != nil {
			return bundled, err
		}
		fileRows, err := db.Query(
			`SELECT file_path, lines, model_id FROM loc_snapshots WHERE snapshot_at=?`, snap,
		)
		if err != nil {
			return bundled, err
		}
		files := map[string]int{}
		var modelID string
		for fileRows.Next() {
			var path string
			var lines int
			var mid string
			if err := fileRows.Scan(&path, &lines, &mid); err != nil {
				fileRows.Close()
				return bundled, err
			}
			files[path] = lines
			modelID = mid
		}
		fileRows.Close()
		payload, _ := json.Marshal(map[string]any{
			"snapshot_at": snap, "files": files, "model_id": modelID, "row_count": len(files),
		})
		archID, _ := storage.NewID()
		if _, err := db.Exec(`
			INSERT INTO archive_entries(id, source_table, source_id, payload_json, archived_at, archive_reason)
			VALUES (?, ?, ?, ?, ?, ?)`,
			archID, "loc_snapshots", snap, string(payload), archivedAt, reason,
		); err != nil {
			return bundled, err
		}
		bundled += len(files)
	}
	if _, err := db.Exec(
		fmt.Sprintf(`DELETE FROM loc_snapshots WHERE snapshot_at NOT IN (%s)`, ph), kept...,
	); err != nil {
		return bundled, err
	}
	return bundled, snapRows.Err()
}

// Entry is a row in archive_entries.
type Entry struct {
	ID             string `json:"id"`
	SourceTable    string `json:"source_table"`
	SourceID       string `json:"source_id"`
	ArchivedAt     string `json:"archived_at"`
	ArchiveReason  string `json:"archive_reason,omitempty"`
}

// ListFilter selects archive entries.
type ListFilter struct {
	Table string
	Since string
	Until string
	Limit int
}

// List returns archive entry metadata.
func List(db *sql.DB, f ListFilter) ([]Entry, error) {
	q := `SELECT id, source_table, source_id, archived_at, COALESCE(archive_reason,'') FROM archive_entries WHERE 1=1`
	args := []any{}
	if f.Table != "" {
		q += ` AND source_table = ?`
		args = append(args, f.Table)
	}
	if f.Since != "" {
		q += ` AND archived_at >= ?`
		args = append(args, f.Since)
	}
	if f.Until != "" {
		q += ` AND archived_at <= ?`
		args = append(args, f.Until)
	}
	q += ` ORDER BY archived_at DESC`
	if f.Limit > 0 {
		q += ` LIMIT ?`
		args = append(args, f.Limit)
	}
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Entry
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.ID, &e.SourceTable, &e.SourceID, &e.ArchivedAt, &e.ArchiveReason); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// RestoreOptions selects rows to restore.
type RestoreOptions struct {
	ID          string
	SourceTable string
	SourceID    string
	Table       string
	Since       string
	Until       string
	KeepArchive bool
}

// RestoreResult summarizes a restore operation.
type RestoreResult struct {
	Restored              int            `json:"restored"`
	SkippedAlreadyPresent int            `json:"skipped_already_present"`
	ByTableRestored       map[string]int `json:"by_table_restored"`
	ByTableSkipped        map[string]int `json:"by_table_skipped"`
	ArchiveEntriesDeleted int            `json:"archive_entries_deleted"`
}

// Restore moves archived rows back to source tables.
func Restore(db *sql.DB, opt RestoreOptions) (RestoreResult, error) {
	if opt.ID == "" && opt.SourceTable == "" && opt.Table == "" && opt.Since == "" && opt.Until == "" && opt.SourceID == "" {
		return RestoreResult{}, fmt.Errorf("must supply at least one selector (--id / --table / --since)")
	}
	q := `SELECT id, source_table, source_id, payload_json, archived_at FROM archive_entries WHERE 1=1`
	args := []any{}
	if opt.ID != "" {
		q += ` AND id = ?`
		args = append(args, opt.ID)
	}
	if opt.SourceTable != "" {
		q += ` AND source_table = ?`
		args = append(args, opt.SourceTable)
	}
	if opt.SourceID != "" {
		q += ` AND source_id = ?`
		args = append(args, opt.SourceID)
	}
	if opt.Table != "" {
		q += ` AND source_table = ?`
		args = append(args, opt.Table)
	}
	if opt.Since != "" {
		q += ` AND archived_at >= ?`
		args = append(args, opt.Since)
	}
	if opt.Until != "" {
		q += ` AND archived_at <= ?`
		args = append(args, opt.Until)
	}

	rows, err := db.Query(q, args...)
	if err != nil {
		return RestoreResult{}, err
	}
	defer rows.Close()

	type candidate struct {
		id, table, payload string
	}
	var candidates []candidate
	for rows.Next() {
		var c candidate
		var srcID, archivedAt string
		if err := rows.Scan(&c.id, &c.table, &srcID, &c.payload, &archivedAt); err != nil {
			return RestoreResult{}, err
		}
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		return RestoreResult{}, err
	}

	order := map[string]int{
		"plan_items": 0, "review_runs": 1,
		"plan_item_acceptance": 2, "plan_item_files": 2, "status_log": 2,
		"review_findings": 3,
	}
	// simple stable sort by table priority
	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			if order[candidates[j].table] < order[candidates[i].table] {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}

	res := RestoreResult{
		ByTableRestored: make(map[string]int),
		ByTableSkipped:  make(map[string]int),
	}
	var deletedIDs []string

	for _, c := range candidates {
		var payload map[string]any
		if err := json.Unmarshal([]byte(c.payload), &payload); err != nil {
			return res, err
		}

		if c.table == "loc_snapshots" {
			n, err := restoreLocSnapshot(db, payload)
			if err != nil {
				return res, err
			}
			if n > 0 {
				res.Restored++
				res.ByTableRestored[c.table]++
				deletedIDs = append(deletedIDs, c.id)
			}
			continue
		}

		cols := make([]string, 0, len(payload))
		vals := make([]any, 0, len(payload))
		for k, v := range payload {
			cols = append(cols, k)
			vals = append(vals, v)
		}
		ph := strings.Repeat("?,", len(cols))
		ph = ph[:len(ph)-1]
		colList := strings.Join(cols, ",")
		result, err := db.Exec(
			fmt.Sprintf("INSERT OR IGNORE INTO %s (%s) VALUES (%s)", c.table, colList, ph),
			vals...,
		)
		if err != nil {
			return res, err
		}
		n, _ := result.RowsAffected()
		if n > 0 {
			res.Restored++
			res.ByTableRestored[c.table]++
			deletedIDs = append(deletedIDs, c.id)
		} else {
			res.SkippedAlreadyPresent++
			res.ByTableSkipped[c.table]++
		}
	}

	if !opt.KeepArchive && len(deletedIDs) > 0 {
		ph := strings.Repeat("?,", len(deletedIDs))
		ph = ph[:len(ph)-1]
		args := make([]any, len(deletedIDs))
		for i, id := range deletedIDs {
			args[i] = id
		}
		if _, err := db.Exec(fmt.Sprintf("DELETE FROM archive_entries WHERE id IN (%s)", ph), args...); err != nil {
			return res, err
		}
		res.ArchiveEntriesDeleted = len(deletedIDs)
	}
	return res, nil
}

func restoreLocSnapshot(db *sql.DB, payload map[string]any) (int, error) {
	exists, err := storage.TableExists(db, "loc_snapshots")
	if err != nil || !exists {
		return 0, nil
	}
	snapAt, _ := payload["snapshot_at"].(string)
	files, _ := payload["files"].(map[string]any)
	modelID, _ := payload["model_id"].(string)
	if modelID == "" {
		modelID = "unknown"
	}
	count := 0
	for path, linesVal := range files {
		lines := 0
		switch v := linesVal.(type) {
		case float64:
			lines = int(v)
		case int:
			lines = v
		}
		id, _ := storage.NewID()
		if _, err := db.Exec(
			`INSERT INTO loc_snapshots (id, snapshot_at, file_path, lines, created_at, model_id) VALUES (?, ?, ?, ?, ?, ?)`,
			id, snapAt, path, lines, snapAt, modelID,
		); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// GCOptions controls garbage collection.
type GCOptions struct {
	OlderThanDays int
	DryRun        bool
}

// GCResult summarizes gc dry-run or execution.
type GCResult struct {
	FeedbackToClose      int `json:"feedback_to_close"`
	FindingsToWontfix    int `json:"findings_to_wontfix"`
	RemindersToArchive   int `json:"reminders_to_archive"`
	TasksToArchive       int `json:"tasks_to_archive"`
	StaleArchNotes       int `json:"stale_arch_notes"`
	OlderThanDays        int `json:"older_than_days"`
	FeedbackClosed       int `json:"feedback_closed,omitempty"`
	FindingsResolved     int `json:"findings_resolved,omitempty"`
	RemindersArchived    int `json:"reminders_archived,omitempty"`
	TasksArchived        int `json:"tasks_archived,omitempty"`
}

// GC prunes stale open feedback, missing-file findings, and old dismissed reminders/tasks.
func GC(db *sql.DB, opt GCOptions) (GCResult, error) {
	if opt.OlderThanDays <= 0 {
		opt.OlderThanDays = 30
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -opt.OlderThanDays).Format(time.RFC3339Nano)

	res := GCResult{OlderThanDays: opt.OlderThanDays}

	fbRows, err := db.Query(
		`SELECT id FROM feedback WHERE status='open' AND created_at < ? ORDER BY created_at`, cutoff,
	)
	if err != nil {
		return res, err
	}
	var fbIDs []string
	for fbRows.Next() {
		var id string
		if err := fbRows.Scan(&id); err != nil {
			fbRows.Close()
			return res, err
		}
		fbIDs = append(fbIDs, id)
	}
	fbRows.Close()
	res.FeedbackToClose = len(fbIDs)

	repoFiles := map[string]bool{}
	rfRows, err := db.Query(`SELECT path FROM repo_files`)
	if err == nil {
		for rfRows.Next() {
			var p string
			if err := rfRows.Scan(&p); err != nil {
				rfRows.Close()
				break
			}
			repoFiles[p] = true
		}
		rfRows.Close()
	}

	var findingIDs []string
	fdRows, err := db.Query(
		`SELECT id, file_path FROM review_findings WHERE status='open' AND file_path IS NOT NULL`,
	)
	if err == nil {
		for fdRows.Next() {
			var id, path string
			if err := fdRows.Scan(&id, &path); err != nil {
				fdRows.Close()
				break
			}
			if len(repoFiles) > 0 && !repoFiles[path] {
				findingIDs = append(findingIDs, id)
			}
		}
		fdRows.Close()
	}
	res.FindingsToWontfix = len(findingIDs)

	remRows, err := db.Query(
		`SELECT * FROM reminders WHERE status='dismissed' AND created_at < ? ORDER BY created_at`, cutoff,
	)
	if err == nil {
		var remCount int
		for remRows.Next() {
			remCount++
		}
		remRows.Close()
		res.RemindersToArchive = remCount
	}

	_ = db.QueryRow(
		`SELECT COUNT(*) FROM tasks WHERE status IN ('done','wontfix') AND created_at < ?`, cutoff,
	).Scan(&res.TasksToArchive)

	res.StaleArchNotes, _ = architecture.CountStale(db)

	if opt.DryRun {
		return res, nil
	}

	archivedAt := storage.NowUTC()
	for _, id := range fbIDs {
		if _, err := db.Exec(
			`UPDATE feedback SET status='closed', proposed_fix=COALESCE(proposed_fix, ?) WHERE id=?`,
			fmt.Sprintf("closed by gc at %s", archivedAt), id,
		); err != nil {
			return res, err
		}
		res.FeedbackClosed++
	}
	for _, id := range findingIDs {
		if _, err := db.Exec(`UPDATE review_findings SET status='wontfix' WHERE id=?`, id); err != nil {
			return res, err
		}
		res.FindingsResolved++
	}

	remPayloads, _ := readTableRows(db, `SELECT * FROM reminders WHERE status='dismissed' AND created_at < ? ORDER BY created_at`, cutoff)
	for _, row := range remPayloads {
		srcID := fmt.Sprint(row.payload["id"])
		if err := archiveRow(db, "reminders", srcID, row.payload, archivedAt,
			fmt.Sprintf("dismissed reminder older than %dd (gc)", opt.OlderThanDays)); err != nil {
			return res, err
		}
		res.RemindersArchived++
	}

	taskPayloads, _ := readTableRows(db, `SELECT * FROM tasks WHERE status IN ('done','wontfix') AND created_at < ? ORDER BY created_at`, cutoff)
	for _, row := range taskPayloads {
		srcID := fmt.Sprint(row.payload["id"])
		if err := archiveRow(db, "tasks", srcID, row.payload, archivedAt,
			fmt.Sprintf("closed task older than %dd (gc)", opt.OlderThanDays)); err != nil {
			return res, err
		}
		res.TasksArchived++
	}

	return res, nil
}

type scannedRow struct {
	payload map[string]any
}

func readTableRows(db *sql.DB, query string, args ...any) ([]scannedRow, error) {
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	cols, err := rows.Columns()
	if err != nil {
		rows.Close()
		return nil, err
	}
	var out []scannedRow
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			rows.Close()
			return nil, err
		}
		payload := make(map[string]any, len(cols))
		for i, c := range cols {
			v := vals[i]
			if b, ok := v.([]byte); ok {
				v = string(b)
			}
			payload[c] = v
		}
		out = append(out, scannedRow{payload: payload})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	return out, nil
}

func archiveRow(db *sql.DB, table, srcID string, payload map[string]any, archivedAt, reason string) error {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	archID, err := storage.NewID()
	if err != nil {
		return err
	}
	if _, err := db.Exec(`
		INSERT INTO archive_entries(id, source_table, source_id, payload_json, archived_at, archive_reason)
		VALUES (?, ?, ?, ?, ?, ?)`,
		archID, table, srcID, string(payloadJSON), archivedAt, reason,
	); err != nil {
		return err
	}
	_, err = db.Exec(fmt.Sprintf("DELETE FROM %s WHERE id=?", table), srcID)
	return err
}
