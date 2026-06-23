package inventory

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/sebihoermann/devdb-go/internal/domain/architecture"
	"github.com/sebihoermann/devdb-go/internal/domain/reminders"
	"github.com/sebihoermann/devdb-go/internal/git"
	"github.com/sebihoermann/devdb-go/internal/storage"
)

// ScanResult summarizes a scan run.
type ScanResult struct {
	RunID        string `json:"run_id"`
	FilesSeen    int    `json:"files_seen"`
	FilesAdded   int    `json:"files_added"`
	FilesChanged int    `json:"files_changed"`
	FilesRemoved int    `json:"files_removed"`
	GitSHA       string `json:"git_sha,omitempty"`
}

type existingFile struct {
	path, kind            string
	language, contentHash sql.NullString
	lines, sizeBytes      sql.NullInt64
}

// Scan updates repo_files and records scan_runs + file_change_events.
// All repository writes (INSERT/UPDATE/DELETE on repo_files, scan_runs, and
// file_change_events) run inside a single transaction; if any write fails the
// scan leaves no partial state behind.
func Scan(db *sql.DB, repoRoot string, paths []string, gitAware bool, modelID string) (ScanResult, error) {
	records, err := ScanInventory(repoRoot, paths, gitAware)
	if err != nil {
		return ScanResult{}, err
	}

	runID, err := storage.NewID()
	if err != nil {
		return ScanResult{}, err
	}
	now := storage.NowUTC()
	sha := git.HeadSHA(repoRoot)
	scope := normalizeScope(paths)

	existing := map[string]existingFile{}
	rows, err := db.Query(`SELECT path, language, kind, lines, content_hash, size_bytes FROM repo_files`)
	if err != nil {
		return ScanResult{}, err
	}
	for rows.Next() {
		var ef existingFile
		if err := rows.Scan(&ef.path, &ef.language, &ef.kind, &ef.lines, &ef.contentHash, &ef.sizeBytes); err != nil {
			rows.Close()
			return ScanResult{}, err
		}
		existing[ef.path] = ef
	}
	rows.Close()

	seen := map[string]bool{}
	var added, changed int
	type changeEvent struct {
		kind, path, oldHash, newHash string
	}
	var events []changeEvent

	for _, rec := range records {
		seen[rec.Path] = true
		prev, ok := existing[rec.Path]

		if !ok {
			added++
			events = append(events, changeEvent{"added", rec.Path, "", rec.ContentHash})
		} else {
			prevLines := int64(0)
			if prev.lines.Valid {
				prevLines = prev.lines.Int64
			}
			recLines := int64(0)
			if rec.Lines != nil {
				recLines = int64(*rec.Lines)
			}
			prevSize := int64(0)
			if prev.sizeBytes.Valid {
				prevSize = prev.sizeBytes.Int64
			}
			prevLang := ""
			if prev.language.Valid {
				prevLang = prev.language.String
			}
			prevHash := ""
			if prev.contentHash.Valid {
				prevHash = prev.contentHash.String
			}
			changedFields := prevLang != rec.Language || prev.kind != rec.Kind ||
				prevLines != recLines || prevHash != rec.ContentHash || prevSize != int64(rec.SizeBytes)
			if changedFields {
				changed++
				events = append(events, changeEvent{"modified", rec.Path, prevHash, rec.ContentHash})
			}
		}
	}

	removed := 0
	for path, row := range existing {
		if seen[path] {
			continue
		}
		if row.kind == "external" {
			continue
		}
		if pathInScope(path, scope) {
			removedHash := ""
			if row.contentHash.Valid {
				removedHash = row.contentHash.String
			}
			events = append(events, changeEvent{"removed", path, removedHash, ""})
		}
		if len(paths) > 0 && !pathInScope(path, scope) {
			continue
		}
		removed++
	}

	err = storage.WithTx(db, func(tx *sql.Tx) error {
		for _, rec := range records {
			_, ok := existing[rec.Path]
			lang := nullStr(rec.Language)
			hash := nullStr(rec.ContentHash)
			var lines any
			if rec.Lines != nil {
				lines = *rec.Lines
			}
			if !ok {
				if _, err := tx.Exec(`
					INSERT INTO repo_files(path, language, kind, lines, content_hash, size_bytes, last_seen_at, last_scan_run_id)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
					rec.Path, lang, rec.Kind, lines, hash, rec.SizeBytes, now, runID,
				); err != nil {
					return err
				}
				continue
			}
			if _, err := tx.Exec(`
				UPDATE repo_files SET language=?, kind=?, lines=?, content_hash=?, size_bytes=?, last_seen_at=?, last_scan_run_id=?
				WHERE path=?`,
				lang, rec.Kind, lines, hash, rec.SizeBytes, now, runID, rec.Path,
			); err != nil {
				return err
			}
		}

		for path, row := range existing {
			if seen[path] {
				continue
			}
			if row.kind == "external" {
				continue
			}
			if len(paths) > 0 && !pathInScope(path, scope) {
				continue
			}
			if _, err := tx.Exec(`DELETE FROM repo_files WHERE path=?`, path); err != nil {
				return err
			}
		}

		if _, err := tx.Exec(`
			INSERT INTO scan_runs(id, started_at, finished_at, git_sha, files_seen, files_added, files_changed, files_removed, model_id)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			runID, now, now, nullStr(sha), len(records), added, changed, removed, modelID,
		); err != nil {
			return err
		}

		for _, ev := range events {
			evID, err := storage.NewID()
			if err != nil {
				return err
			}
			if _, err := tx.Exec(`
				INSERT INTO file_change_events(id, scan_run_id, path, change_kind, old_hash, new_hash, created_at, model_id)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
				evID, runID, ev.path, ev.kind, nullStr(ev.oldHash), nullStr(ev.newHash), now, modelID,
			); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return ScanResult{}, err
	}

	return ScanResult{
		RunID: runID, FilesSeen: len(records), FilesAdded: added,
		FilesChanged: changed, FilesRemoved: removed, GitSHA: sha,
	}, nil
}

// LocSummary aggregates line counts from a scan (not persisted).
type LocSummary struct {
	Files      int            `json:"files"`
	TotalLines int            `json:"total_lines"`
	ByKind     map[string]int `json:"by_kind"`
	ByLanguage map[string]int `json:"by_language"`
}

// Loc scans files and returns line-count aggregates.
func Loc(repoRoot string, paths []string, gitAware bool) (LocSummary, error) {
	records, err := ScanInventory(repoRoot, paths, gitAware)
	if err != nil {
		return LocSummary{}, err
	}
	sum := LocSummary{ByKind: map[string]int{}, ByLanguage: map[string]int{}}
	for _, rec := range records {
		if rec.Lines == nil {
			continue
		}
		sum.Files++
		sum.TotalLines += *rec.Lines
		sum.ByKind[rec.Kind] += *rec.Lines
		if rec.Language != "" {
			sum.ByLanguage[rec.Language] += *rec.Lines
		}
	}
	return sum, nil
}

// Freshness is last scan metadata for report reads.
type Freshness struct {
	LastScanAt   string `json:"last_scan_at,omitempty"`
	FilesIndexed int    `json:"files_indexed"`
	ChangedSince int    `json:"changed_since_last_scan"`
}

// FreshnessInfo reads scan freshness from the database.
func FreshnessInfo(db *sql.DB) (Freshness, error) {
	var f Freshness
	_ = db.QueryRow(`SELECT COUNT(*) FROM repo_files`).Scan(&f.FilesIndexed)

	var lastRunID, finishedAt sql.NullString
	err := db.QueryRow(`
		SELECT id, finished_at FROM scan_runs ORDER BY finished_at DESC LIMIT 1`,
	).Scan(&lastRunID, &finishedAt)
	if err == sql.ErrNoRows {
		return f, nil
	}
	if err != nil {
		return f, err
	}
	f.LastScanAt = finishedAt.String

	if lastRunID.Valid {
		var prevFinished sql.NullString
		_ = db.QueryRow(`
			SELECT finished_at FROM scan_runs WHERE id != ? ORDER BY finished_at DESC LIMIT 1`,
			lastRunID.String,
		).Scan(&prevFinished)
		if prevFinished.Valid {
			_ = db.QueryRow(`
				SELECT COUNT(*) FROM file_change_events
				WHERE scan_run_id=? AND change_kind IN ('added','modified')`,
				lastRunID.String,
			).Scan(&f.ChangedSince)
		}
	}
	return f, nil
}

// ContextPayload is focused pre-edit context.
type ContextPayload struct {
	Task              string              `json:"task,omitempty"`
	Files             []string            `json:"files"`
	PlanItemID        string              `json:"plan_item_id,omitempty"`
	StaleNotes        []architecture.Note `json:"stale_notes"`
	ArchitectureNotes []architecture.Note `json:"architecture_notes"`
	OpenFindings      []FindingSummary    `json:"open_findings"`
	FileReminders     []reminders.Row     `json:"file_reminders"`
	PlanReminders     []reminders.Row     `json:"plan_reminders"`
}

// FindingSummary is a compact review finding row (M6 fills this; empty until then).
type FindingSummary struct {
	ID        string `json:"id"`
	FilePath  string `json:"file_path,omitempty"`
	LineStart *int   `json:"line_start,omitempty"`
	Severity  string `json:"severity"`
	Title     string `json:"title"`
}

// ContextOptions configures a context read.
type ContextOptions struct {
	Files      []string
	Task       string
	PlanItemID string
}

// Context assembles notes, findings, and reminders for files about to be edited.
func Context(db *sql.DB, opt ContextOptions) (ContextPayload, error) {
	payload := ContextPayload{
		Files:             opt.Files,
		Task:              opt.Task,
		PlanItemID:        opt.PlanItemID,
		StaleNotes:        []architecture.Note{},
		ArchitectureNotes: []architecture.Note{},
		OpenFindings:      []FindingSummary{},
		FileReminders:     []reminders.Row{},
		PlanReminders:     []reminders.Row{},
	}

	noteSeen := map[string]bool{}
	staleSeen := map[string]bool{}

	addNotes := func(notes []architecture.Note) {
		for _, n := range notes {
			if noteSeen[n.ID] {
				continue
			}
			noteSeen[n.ID] = true
			payload.ArchitectureNotes = append(payload.ArchitectureNotes, n)
			if n.Stale || n.Status == "stale" {
				if !staleSeen[n.ID] {
					staleSeen[n.ID] = true
					payload.StaleNotes = append(payload.StaleNotes, n)
				}
			}
		}
	}

	if len(opt.Files) == 0 {
		active, err := architecture.List(db, architecture.ListFilter{Status: "active", Limit: 5})
		if err != nil {
			return payload, err
		}
		addNotes(active)
		stale, err := architecture.List(db, architecture.ListFilter{Stale: true, Limit: 5})
		if err != nil {
			return payload, err
		}
		for _, n := range stale {
			if !staleSeen[n.ID] {
				staleSeen[n.ID] = true
				payload.StaleNotes = append(payload.StaleNotes, n)
			}
		}
	} else {
		for _, filePath := range opt.Files {
			touching, err := architecture.List(db, architecture.ListFilter{TouchingPath: filePath})
			if err != nil {
				return payload, err
			}
			addNotes(touching)
		}
	}

	findings, err := listFindingsForFiles(db, opt.Files, 10)
	if err != nil {
		return payload, err
	}
	payload.OpenFindings = findings

	if len(opt.Files) > 0 {
		seen := map[string]bool{}
		for _, filePath := range opt.Files {
			rows, err := reminders.ListForFile(db, filePath)
			if err != nil {
				return payload, err
			}
			for _, r := range rows {
				if !seen[r.ID] {
					seen[r.ID] = true
					payload.FileReminders = append(payload.FileReminders, r)
				}
			}
		}
	}
	if opt.PlanItemID != "" {
		rows, err := reminders.ListForPlanItem(db, opt.PlanItemID)
		if err != nil {
			return payload, err
		}
		payload.PlanReminders = rows
	}

	return payload, nil
}

func listFindingsForFiles(db *sql.DB, files []string, limit int) ([]FindingSummary, error) {
	exists, err := storage.TableExists(db, "review_findings")
	if err != nil || !exists {
		return nil, err
	}
	if limit <= 0 {
		limit = 10
	}
	var out []FindingSummary
	if len(files) == 0 {
		rows, err := db.Query(`
			SELECT id, COALESCE(file_path,''), line_start, severity, title
			FROM review_findings WHERE status='open' ORDER BY created_at DESC LIMIT ?`, limit)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var f FindingSummary
			var line sql.NullInt64
			if err := rows.Scan(&f.ID, &f.FilePath, &line, &f.Severity, &f.Title); err != nil {
				return nil, err
			}
			if line.Valid {
				v := int(line.Int64)
				f.LineStart = &v
			}
			out = append(out, f)
		}
		return out, rows.Err()
	}
	seen := map[string]bool{}
	for _, filePath := range files {
		rows, err := db.Query(`
			SELECT id, COALESCE(file_path,''), line_start, severity, title
			FROM review_findings WHERE status='open' AND file_path=? LIMIT ?`, filePath, limit)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var f FindingSummary
			var line sql.NullInt64
			if err := rows.Scan(&f.ID, &f.FilePath, &line, &f.Severity, &f.Title); err != nil {
				rows.Close()
				return nil, err
			}
			if line.Valid {
				v := int(line.Int64)
				f.LineStart = &v
			}
			if !seen[f.ID] {
				seen[f.ID] = true
				out = append(out, f)
			}
		}
		rows.Close()
	}
	return out, nil
}

// DiffRow is one changed file with linked notes/findings.
type DiffRow struct {
	Path              string   `json:"path"`
	ArchitectureNotes []string `json:"architecture_notes"`
	OpenFindings      []string `json:"open_findings"`
}

// DiffSince compares git ref to working tree and lists linked ledger rows.
func DiffSince(db *sql.DB, repoRoot, ref string) ([]DiffRow, error) {
	changed, err := git.DiffNameOnly(repoRoot, ref)
	if err != nil {
		return nil, fmt.Errorf("unable to diff against %s: %w", ref, err)
	}
	var rows []DiffRow
	for _, path := range changed {
		row := DiffRow{Path: path}
		notes, err := architecture.List(db, architecture.ListFilter{TouchingPath: path})
		if err != nil {
			return nil, err
		}
		for _, n := range notes {
			row.ArchitectureNotes = append(row.ArchitectureNotes, n.Topic)
		}
		findings, err := listFindingsForFiles(db, []string{path}, 5)
		if err != nil {
			return nil, err
		}
		for _, f := range findings {
			row.OpenFindings = append(row.OpenFindings, f.Title)
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func normalizeScope(paths []string) []string {
	if len(paths) == 0 {
		return []string{"."}
	}
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = strings.TrimSuffix(filepathToSlash(p), "/")
	}
	return out
}

func filepathToSlash(p string) string {
	return strings.TrimPrefix(strings.ReplaceAll(p, "\\", "/"), "./")
}

func pathInScope(path string, scope []string) bool {
	for _, item := range scope {
		if item == "." || item == "./" || item == "" {
			return true
		}
		if path == item || strings.HasPrefix(path, item+"/") {
			return true
		}
	}
	return false
}

func nullStr(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

// FormatContextHuman renders compact context output.
func FormatContextHuman(p ContextPayload) []string {
	lines := []string{"# context"}
	if p.Task != "" {
		lines = append(lines, "task: "+p.Task)
	}
	if len(p.Files) > 0 {
		lines = append(lines, "files: "+strings.Join(p.Files, ", "))
	}
	lines = append(lines, "")

	if len(p.FileReminders) > 0 {
		lines = append(lines, "## reminders (file)")
		for _, r := range p.FileReminders {
			lines = append(lines, formatReminderLine(r))
		}
		lines = append(lines, "")
	}
	if len(p.PlanReminders) > 0 {
		lines = append(lines, "## reminders (plan)")
		for _, r := range p.PlanReminders {
			lines = append(lines, formatReminderLine(r))
		}
		lines = append(lines, "")
	}

	lines = append(lines, "## stale warnings")
	if len(p.StaleNotes) == 0 {
		lines = append(lines, "- none")
	} else {
		for _, n := range p.StaleNotes {
			lines = append(lines, fmt.Sprintf("- %s · %s", n.Topic, strings.Join(n.SourcePaths, ", ")))
		}
	}
	lines = append(lines, "", "## architecture notes")
	if len(p.ArchitectureNotes) == 0 {
		lines = append(lines, "- none")
	} else {
		limit := len(p.ArchitectureNotes)
		if limit > 5 {
			limit = 5
		}
		for _, n := range p.ArchitectureNotes[:limit] {
			lines = append(lines, fmt.Sprintf("- %s: %s", n.Topic, summarizeBody(n.Body, 2)))
		}
	}
	lines = append(lines, "", "## open findings")
	if len(p.OpenFindings) == 0 {
		lines = append(lines, "- none")
	} else {
		limit := len(p.OpenFindings)
		if limit > 10 {
			limit = 10
		}
		for _, f := range p.OpenFindings[:limit] {
			loc := f.FilePath
			if loc == "" {
				loc = "cross-cutting"
			}
			if f.LineStart != nil {
				loc = fmt.Sprintf("%s:%d", loc, *f.LineStart)
			}
			lines = append(lines, fmt.Sprintf("- %s · %s · %s", loc, f.Severity, f.Title))
		}
	}
	return lines
}

func formatReminderLine(r reminders.Row) string {
	prefix := ""
	if reminders.IsOverdue(r) {
		prefix = "!! OVERDUE "
	}
	due := ""
	if r.DueAt != "" && len(r.DueAt) >= 10 {
		due = " due:" + r.DueAt[:10]
	}
	return fmt.Sprintf("- %s[%s]%s %s", prefix, r.ID[:8], due, r.Title)
}

func summarizeBody(body string, maxLines int) string {
	body = strings.TrimSpace(body)
	parts := strings.Split(body, "\n")
	if len(parts) <= maxLines {
		return strings.Join(parts, " ")
	}
	return strings.Join(parts[:maxLines], " ") + "..."
}

// ContextStrictExit returns true when strict mode should exit non-zero.
func ContextStrictExit(p ContextPayload) bool {
	for _, f := range p.OpenFindings {
		if f.Severity == "high" || f.Severity == "critical" {
			return true
		}
	}
	return len(p.StaleNotes) > 0
}
