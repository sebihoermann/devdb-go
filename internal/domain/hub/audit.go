package hub

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/sebihoermann/devdb-go/internal/domain/architecture"
	"github.com/sebihoermann/devdb-go/internal/migrate"
	"github.com/sebihoermann/devdb-go/internal/storage"
)

// auditSectionOrder is the canonical render order for sections.
var auditSectionOrder = []string{
	"high_feedback",
	"high_findings",
	"stale_arch",
	"overdue_reminders",
	"in_progress",
	"blocked",
	"planned_per_project",
	"stale_verification",
}

// auditKindAliases maps short user-facing kind names to canonical
// section names. Both are accepted by AuditOptions.Kinds.
var auditKindAliases = map[string]string{
	"feedback":      "high_feedback",
	"findings":      "high_findings",
	"stale_arch":    "stale_arch",
	"overdue":       "overdue_reminders",
	"in_progress":   "in_progress",
	"blocked":       "blocked",
	"planned":       "planned_per_project",
	"verification":  "stale_verification",
	"high_feedback": "high_feedback",
	"high_findings": "high_findings",
	"overdue_reminders":  "overdue_reminders",
	"planned_per_project": "planned_per_project",
	"stale_verification":  "stale_verification",
}

func canonicalKind(k string) string {
	if v, ok := auditKindAliases[strings.ToLower(strings.TrimSpace(k))]; ok {
		return v
	}
	return k
}

// CanonicalKindForTest exposes canonicalKind for black-box tests.
func CanonicalKindForTest(k string) string { return canonicalKind(k) }

func isKnownSeverity(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "info", "low", "med", "medium", "high", "critical":
		return true
	}
	return false
}

// AuditSectionOrder returns the canonical render order for sections.
func AuditSectionOrder() []string {
	out := make([]string, len(auditSectionOrder))
	copy(out, auditSectionOrder)
	return out
}

// blockedKeywords is the lower-cased substring set used to detect blocked
// plan items from their most recent in_progress status_log note.
var blockedKeywords = []string{
	"blocked", "blocker", "waiting", "stuck", "cannot proceed", "can't proceed",
}

// AuditSection is one named section of an AuditReport.
//
// Rows is a list of {project, ...} records; the per-row keys are section-specific.
type AuditSection struct {
	Kind string           `json:"kind"`
	Rows []map[string]any `json:"rows"`
}

// AuditOptions filters an Audit run.
type AuditOptions struct {
	// Severity is the floor for the high_feedback and high_findings sections.
	// Accepts: info|low|med|medium|high|critical. Empty defaults to "high".
	Severity string
	// Kinds is a whitelist of section names to include. Empty = all sections.
	Kinds []string
	// Projects narrows the registry walk. Empty = all projects.
	Projects []string
	// Mode is "live" (default) for direct reads from each project's
	// .devdb/development.db, or "cached" to read from ~/.devdb/metadata.db.
	Mode string
	// IncludeArchived surfaces archived feedback rows in the high_feedback
	// section. Default false.
	IncludeArchived bool
	// Registry path override (default: ~/.devdb-projects).
	Registry string
	// MetadataDB path override (default: ~/.devdb/metadata.db).
	MetadataDB string
}

// AuditReport is the cross-project audit snapshot.
type AuditReport struct {
	CollectedAt       string                    `json:"collected_at"`
	Registry          string                    `json:"registry"`
	Mode              string                    `json:"mode"`
	SeverityThreshold string                    `json:"severity_threshold"`
	Sections          map[string]AuditSection   `json:"sections"`
	ByProject         map[string]map[string]int `json:"by_project"`
}

// Audit runs the cross-project audit and returns the report.
//
// Live mode opens each project's .devdb/development.db directly and
// queries feedback / review_findings / architecture_notes / reminders /
// plan_items / status_log / verification_runs. Corrupt, missing, or
// legacy-Python databases are skipped silently, matching Across behaviour.
//
// Cached mode reads from ~/.devdb/metadata.db project_snapshots and
// returns only counts (per-section rows are empty in cached mode
// because the snapshot does not carry row-level data).
func Audit(opts AuditOptions) (AuditReport, error) {
	if opts.Severity == "" {
		opts.Severity = "high"
	}
	if !isKnownSeverity(opts.Severity) {
		return AuditReport{}, fmt.Errorf("invalid severity %q (try: info|low|med|medium|high|critical)", opts.Severity)
	}
	threshold := severityRank(opts.Severity)
	if opts.Mode == "" {
		opts.Mode = "live"
	}
	if opts.Mode != "live" && opts.Mode != "cached" {
		return AuditReport{}, fmt.Errorf("invalid mode %q (try: live|cached)", opts.Mode)
	}
	registryPath := ResolveRegistry(opts.Registry)
	report := AuditReport{
		CollectedAt:       storage.NowUTC(),
		Registry:          registryPath,
		Mode:              opts.Mode,
		SeverityThreshold: opts.Severity,
		Sections:          map[string]AuditSection{},
		ByProject:         map[string]map[string]int{},
	}
	for _, kind := range auditSectionOrder {
		report.Sections[kind] = AuditSection{Kind: kind, Rows: nil}
	}
	wanted := map[string]bool{}
	for _, k := range opts.Kinds {
		for _, p := range strings.Split(k, ",") {
			p = canonicalKind(p)
			if p != "" {
				wanted[p] = true
			}
		}
	}
	include := func(kind string) bool {
		if len(wanted) == 0 {
			return true
		}
		return wanted[kind]
	}

	if opts.Mode == "cached" {
		if err := auditCached(&report, opts, include); err != nil {
			return report, err
		}
	} else {
		if err := auditLive(&report, registryPath, threshold, include, opts); err != nil {
			return report, err
		}
	}

	for k := range report.Sections {
		sortSectionRows(report.Sections[k])
	}
	return report, nil
}

func auditLive(report *AuditReport, registryPath string, threshold int, include func(string) bool, opts AuditOptions) error {
	projects, err := ReadRegistry(registryPath)
	if err != nil {
		return err
	}
	for _, p := range projects {
		if !projectMatches(p.Alias, opts.Projects) {
			continue
		}
		if !p.Exists {
			ensureProjectCounts(report, p.Alias)
			continue
		}
		db, err := storage.Open(p.DBPath)
		if err != nil {
			ensureProjectCounts(report, p.Alias)
			continue
		}
		kind, _, err := storage.DetectSchema(db)
		if err != nil || kind == storage.SchemaPython {
			db.Close()
			ensureProjectCounts(report, p.Alias)
			continue
		}
		if err := migrate.RunAll(db); err != nil {
			db.Close()
			ensureProjectCounts(report, p.Alias)
			continue
		}
		auditOneProject(db, p.Alias, report, threshold, include, opts.IncludeArchived)
		db.Close()
	}
	return nil
}

func auditCached(report *AuditReport, opts AuditOptions, include func(string) bool) error {
	entries, err := List(opts.Registry, opts.MetadataDB, false)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !projectMatches(e.Alias, opts.Projects) {
			continue
		}
		if e.Snapshot == nil {
			ensureProjectCounts(report, e.Alias)
			continue
		}
		s := *e.Snapshot
		mergeSnapshotIntoReport(report, e.Alias, s, severityRank(opts.Severity), include)
	}
	return nil
}

func mergeSnapshotIntoReport(report *AuditReport, alias string, s Snapshot, threshold int, include func(string) bool) {
	counts := ensureProjectCounts(report, alias)
	counts["open_high_feedback"] = s.OpenHighFeedback
	counts["open_high_findings"] = s.OpenHighFindings
	counts["open_plan_items"] = s.OpenPlanItems
	counts["in_progress"] = s.InProgressItems
	counts["stale_arch"] = s.StaleArchNotes
	counts["overdue_reminders"] = s.OverdueReminders
	if s.LatestVerificationFreshness != "" && s.LatestVerificationFreshness != "fresh" && s.LatestVerificationStatus == "passed" {
		if include("stale_verification") {
			appendSectionRow(report, "stale_verification", map[string]any{
				"project":  alias,
				"command":  s.LatestVerificationCommand,
				"scope":    s.LatestVerificationScope,
				"finished": s.LatestVerificationAt,
				"reason":   s.LatestVerificationFreshness,
			})
		}
	}
}

func auditOneProject(db *sql.DB, alias string, report *AuditReport, threshold int, include func(string) bool, includeArchived bool) {
	counts := ensureProjectCounts(report, alias)
	sevList := severityListAtOrAbove(threshold)
	if sevList == "" {
		return
	}

	if include("high_feedback") {
		rows, err := db.Query(fmt.Sprintf(`
			SELECT id, severity, category, note, created_at
			FROM feedback
			WHERE status='open' AND severity IN (%s)
			ORDER BY created_at DESC`, sevList))
		if err == nil {
			for rows.Next() {
				var id, severity, note, createdAt string
				var category sql.NullString
				if err := rows.Scan(&id, &severity, &category, &note, &createdAt); err != nil {
					continue
				}
				catStr := ""
				if category.Valid {
					catStr = category.String
				}
				appendSectionRow(report, "high_feedback", map[string]any{
					"project":    alias,
					"id":         id,
					"id_prefix":  idPrefix(id, 8),
					"severity":   severity,
					"category":   catStr,
					"note":       truncate(note, 200),
					"created_at": createdAt,
				})
				counts["open_high_feedback"]++
			}
			rows.Close()
		}
	}

	if include("high_findings") {
		rows, err := db.Query(fmt.Sprintf(`
			SELECT id, severity, title, file_path, principle, created_at
			FROM review_findings
			WHERE status='open' AND severity IN (%s)
			ORDER BY created_at DESC`, sevList))
		if err == nil {
			for rows.Next() {
				var id, severity, title, filePath, principle, createdAt string
				if err := rows.Scan(&id, &severity, &title, &filePath, &principle, &createdAt); err != nil {
					continue
				}
				appendSectionRow(report, "high_findings", map[string]any{
					"project":    alias,
					"id":         id,
					"id_prefix":  idPrefix(id, 8),
					"severity":   severity,
					"title":      title,
					"file_path":  filePath,
					"principle":  principle,
					"created_at": createdAt,
				})
				counts["open_high_findings"]++
			}
			rows.Close()
		}
	}

	if include("stale_arch") {
		notes, err := architecture.List(db, architecture.ListFilter{Status: "active"})
		if err == nil {
			for _, n := range notes {
				if !n.Stale {
					continue
				}
				appendSectionRow(report, "stale_arch", map[string]any{
					"project":   alias,
					"id":        n.ID,
					"id_prefix": idPrefix(n.ID, 8),
					"topic":     n.Topic,
				})
				counts["stale_arch"]++
			}
		}
		row := db.QueryRow(`SELECT COUNT(*) FROM architecture_notes WHERE status='stale'`)
		var markedStale int
		_ = row.Scan(&markedStale)
		counts["stale_arch"] += markedStale
	}

	if include("overdue_reminders") {
		rows, err := db.Query(`
			SELECT id, title, due_at, plan_item_id, file_path
			FROM reminders
			WHERE status='open' AND due_at IS NOT NULL AND due_at < datetime('now')
			ORDER BY due_at ASC`)
		if err == nil {
			for rows.Next() {
				var id, title, dueAt, planItemID, filePath sql.NullString
				if err := rows.Scan(&id, &title, &dueAt, &planItemID, &filePath); err != nil {
					continue
				}
				appendSectionRow(report, "overdue_reminders", map[string]any{
					"project":      alias,
					"id":           id.String,
					"id_prefix":    idPrefix(id.String, 8),
					"title":        title.String,
					"due_at":       dueAt.String,
					"plan_item_id": planItemID.String,
					"file_path":    filePath.String,
				})
				counts["overdue_reminders"]++
			}
			rows.Close()
		}
	}

	if include("in_progress") {
		rows, err := db.Query(`
			SELECT id, title, plan_id, milestone_id
			FROM plan_items
			WHERE status='in_progress'
			ORDER BY created_at DESC`)
		if err == nil {
			for rows.Next() {
				var id, title, planID, milestoneID sql.NullString
				if err := rows.Scan(&id, &title, &planID, &milestoneID); err != nil {
					continue
				}
				appendSectionRow(report, "in_progress", map[string]any{
					"project":      alias,
					"id":           id.String,
					"id_prefix":    idPrefix(id.String, 8),
					"title":        title.String,
					"plan_id":      planID.String,
					"milestone_id": milestoneID.String,
				})
				counts["in_progress"]++
			}
			rows.Close()
		}
	}

	if include("blocked") {
		rows, err := db.Query(`
			SELECT pi.id, pi.title, sl.note, pi.plan_id, pi.milestone_id
			FROM plan_items pi
			JOIN status_log sl ON sl.plan_item_id=pi.id
			WHERE pi.status='in_progress'
			  AND sl.status='in_progress'
			  AND sl.created_at = (
			    SELECT MAX(sl2.created_at) FROM status_log sl2
			    WHERE sl2.plan_item_id=pi.id AND sl2.status='in_progress'
			  )
			  AND (LOWER(sl.note) LIKE '%blocked%'
			    OR LOWER(sl.note) LIKE '%blocker%'
			    OR LOWER(sl.note) LIKE '%waiting%'
			    OR LOWER(sl.note) LIKE '%stuck%'
			    OR LOWER(sl.note) LIKE '%cannot proceed%'
			    OR LOWER(sl.note) LIKE '%can''t proceed%')
			ORDER BY sl.created_at DESC`)
		if err == nil {
			for rows.Next() {
				var id, title, note, planID, milestoneID sql.NullString
				if err := rows.Scan(&id, &title, &note, &planID, &milestoneID); err != nil {
					continue
				}
				appendSectionRow(report, "blocked", map[string]any{
					"project":      alias,
					"id":           id.String,
					"id_prefix":    idPrefix(id.String, 8),
					"title":        title.String,
					"note":         note.String,
					"plan_id":      planID.String,
					"milestone_id": milestoneID.String,
				})
			}
			rows.Close()
		}
	}

	if include("planned_per_project") {
		plannedCount := 0
		var firstID, firstTitle sql.NullString
		_ = db.QueryRow(`SELECT COUNT(*) FROM plan_items WHERE status='planned'`).Scan(&plannedCount)
		_ = db.QueryRow(`
			SELECT id, title FROM plan_items
			WHERE status='planned'
			ORDER BY created_at ASC LIMIT 1`).Scan(&firstID, &firstTitle)
		counts["open_plan_items"] = plannedCount
		row := map[string]any{
			"project": alias,
			"count":   plannedCount,
		}
		if firstID.Valid && firstTitle.Valid {
			row["next_id"] = firstID.String
			row["next_id_prefix"] = idPrefix(firstID.String, 8)
			row["next"] = firstTitle.String
		}
		appendSectionRow(report, "planned_per_project", row)
	}

	if include("stale_verification") {
		status, freshness, command, scope, finishedAt := latestVerification(db)
		if status == "passed" && freshness != "" && freshness != "fresh" {
			appendSectionRow(report, "stale_verification", map[string]any{
				"project":  alias,
				"command":  command,
				"scope":    scope,
				"finished": finishedAt,
				"reason":   freshness,
			})
		}
	}
}

func ensureProjectCounts(report *AuditReport, alias string) map[string]int {
	if report.ByProject[alias] == nil {
		report.ByProject[alias] = map[string]int{}
	}
	return report.ByProject[alias]
}

func projectMatches(alias string, filter []string) bool {
	if len(filter) == 0 {
		return true
	}
	for _, f := range filter {
		if f == alias {
			return true
		}
	}
	return false
}

func severityListAtOrAbove(threshold int) string {
	all := []struct {
		name string
		rank int
	}{
		{"info", 1}, {"low", 2}, {"med", 3}, {"medium", 3}, {"high", 4}, {"critical", 5},
	}
	var names []string
	for _, s := range all {
		if s.rank >= threshold {
			names = append(names, "'"+s.name+"'")
		}
	}
	return strings.Join(names, ",")
}

func appendSectionRow(report *AuditReport, kind string, row map[string]any) {
	sec := report.Sections[kind]
	sec.Rows = append(sec.Rows, row)
	report.Sections[kind] = sec
}

func sortSectionRows(s AuditSection) {
	rows := s.Rows
	switch s.Kind {
	case "high_feedback", "high_findings":
		sort.Slice(rows, func(i, j int) bool {
			si := severityRank(fmt.Sprint(rows[i]["severity"]))
			sj := severityRank(fmt.Sprint(rows[j]["severity"]))
			if si != sj {
				return si > sj
			}
			pi := fmt.Sprint(rows[i]["project"])
			pj := fmt.Sprint(rows[j]["project"])
			if pi != pj {
				return pi < pj
			}
			return fmt.Sprint(rows[i]["created_at"]) > fmt.Sprint(rows[j]["created_at"])
		})
	default:
		sort.Slice(rows, func(i, j int) bool {
			pi := fmt.Sprint(rows[i]["project"])
			pj := fmt.Sprint(rows[j]["project"])
			if pi != pj {
				return pi < pj
			}
			if ti, ok := rows[i]["title"]; ok {
				if tj, ok2 := rows[j]["title"]; ok2 {
					return fmt.Sprint(ti) < fmt.Sprint(tj)
				}
			}
			if ti, ok := rows[i]["topic"]; ok {
				if tj, ok2 := rows[j]["topic"]; ok2 {
					return fmt.Sprint(ti) < fmt.Sprint(tj)
				}
			}
			return fmt.Sprint(rows[i]["id_prefix"]) < fmt.Sprint(rows[j]["id_prefix"])
		})
	}
	s.Rows = rows
}

func idPrefix(id string, n int) string {
	if len(id) <= n {
		return id
	}
	return id[:n]
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
