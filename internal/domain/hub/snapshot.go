package hub

import (
	"database/sql"
	"encoding/json"
	"os"
	"strings"
	"time"

	"github.com/sebihoermann/devdb-go/internal/domain/planning"
	"github.com/sebihoermann/devdb-go/internal/domain/status"
	"github.com/sebihoermann/devdb-go/internal/domain/verification"
	"github.com/sebihoermann/devdb-go/internal/git"
	"github.com/sebihoermann/devdb-go/internal/migrate"
	"github.com/sebihoermann/devdb-go/internal/storage"
)

// AttentionItem is a dashboard attention row.
type AttentionItem struct {
	Kind     string `json:"kind"`
	Severity string `json:"severity"`
	Title    string `json:"title"`
	Status   string `json:"status"`
	Target   string `json:"target,omitempty"`
}

// Snapshot is the hub projection for one project.
type Snapshot struct {
	CollectedAt   string `json:"collected_at"`
	SyncStatus    string `json:"sync_status"`
	Error         string `json:"error,omitempty"`
	SourceDBMTime string `json:"source_db_mtime,omitempty"`

	GitBranch string `json:"git_branch,omitempty"`
	GitSHA    string `json:"git_sha,omitempty"`
	GitDirty  bool   `json:"git_dirty"`
	GitAhead  int    `json:"git_ahead"`
	GitBehind int    `json:"git_behind"`

	WorkStatus       string `json:"work_status"`
	StatusReason     string `json:"status_reason,omitempty"`
	InProgressItems  int    `json:"in_progress_plan_items"`
	OpenPlanItems    int    `json:"open_plan_items"`
	OpenFeedback     int    `json:"open_feedback"`
	InFlightTitle    string `json:"in_flight_title,omitempty"`
	BlockedReason    string `json:"blocked_reason,omitempty"`

	OpenHighFeedback int `json:"open_high_feedback"`
	StaleArchNotes   int `json:"stale_arch_notes"`
	OpenFindings     int `json:"open_findings"`
	OpenHighFindings int `json:"open_high_findings"`
	MissedCalls7d    int `json:"missed_calls_7d"`
	OverdueReminders int `json:"overdue_reminders"`

	LatestVerificationStatus    string `json:"latest_verification_status,omitempty"`
	LatestVerificationFreshness string `json:"latest_verification_freshness,omitempty"`
	LatestVerificationCommand   string `json:"latest_verification_command,omitempty"`
	LatestVerificationScope     string `json:"latest_verification_scope,omitempty"`
	LatestVerificationAt        string `json:"latest_verification_finished_at,omitempty"`

	FilesIndexed int `json:"files_indexed,omitempty"`
	FilesChanged int `json:"files_changed,omitempty"`

	AttentionScore  int             `json:"attention_score"`
	AttentionItems  []AttentionItem `json:"attention_items,omitempty"`
}

// CollectSnapshot reads project state from disk and database.
func CollectSnapshot(root, dbPath string) Snapshot {
	now := storage.NowUTC()
	snap := Snapshot{
		CollectedAt: now,
		SyncStatus:  "active",
		WorkStatus:  "idle",
	}
	root = expandHome(root)
	dbPath = expandHome(dbPath)

	if info, err := os.Stat(dbPath); err == nil {
		snap.SourceDBMTime = info.ModTime().UTC().Format(time.RFC3339Nano)
	} else if os.IsNotExist(err) {
		snap.SyncStatus = "missing"
		snap.WorkStatus = "attention"
		snap.Error = "database not found: " + dbPath
		snap.StatusReason = snap.Error
		snap.AttentionScore = 100
		snap.AttentionItems = []AttentionItem{{
			Kind: "missing_db", Severity: "critical", Title: snap.Error, Status: "open",
		}}
		return snap
	}

	snap.GitBranch = git.Branch(root)
	snap.GitSHA = git.HeadSHA(root)
	snap.GitDirty = git.IsDirty(root)
	snap.GitAhead, snap.GitBehind = git.AheadBehind(root)

	db, err := storage.Open(dbPath)
	if err != nil {
		snap.SyncStatus = "error"
		snap.WorkStatus = "attention"
		snap.Error = err.Error()
		snap.StatusReason = snap.Error
		snap.AttentionScore = 90
		snap.AttentionItems = []AttentionItem{{
			Kind: "project_error", Severity: "critical", Title: snap.Error, Status: "open",
		}}
		return snap
	}
	defer db.Close()

	kind, version, err := storage.DetectSchema(db)
	if err != nil || kind == storage.SchemaPython {
		snap.SyncStatus = "error"
		snap.WorkStatus = "attention"
		snap.Error = "legacy or unreadable database"
		snap.StatusReason = snap.Error
		snap.AttentionScore = 80
		return snap
	}
	if kind == storage.SchemaGo || kind == storage.SchemaUnknown {
		_ = migrate.RunAll(db)
	}

	st, err := status.Build(db, root, string(kind), version)
	if err == nil {
		snap.OpenPlanItems = st.OpenItems
		snap.InProgressItems = st.InProgress
		snap.OpenFeedback = st.OpenFeedback
		snap.WorkStatus = strings.Split(st.Overall, " ·")[0]
		if st.InFlight != nil {
			snap.InFlightTitle = st.InFlight.Title
		}
	}
	q, _ := status.Quality(db)
	snap.OpenHighFeedback = q.OpenHighFeedback
	snap.StaleArchNotes = q.StaleArchNotes
	snap.OpenFindings = q.OpenFindings
	snap.MissedCalls7d = q.MissedCalls7d

	_ = db.QueryRow(`
		SELECT COUNT(*) FROM review_findings
		WHERE status='open' AND severity IN ('high','critical')`).Scan(&snap.OpenHighFindings)

	_ = db.QueryRow(`
		SELECT COUNT(*) FROM reminders
		WHERE status='open' AND due_at IS NOT NULL AND due_at < datetime('now')`).Scan(&snap.OverdueReminders)

	var scanFinished sql.NullString
	var filesSeen, filesChanged sql.NullInt64
	if err := db.QueryRow(`
		SELECT finished_at, files_seen, files_changed FROM scan_runs
		ORDER BY started_at DESC LIMIT 1`).Scan(&scanFinished, &filesSeen, &filesChanged); err == nil {
		if filesSeen.Valid {
			snap.FilesIndexed = int(filesSeen.Int64)
		}
		if filesChanged.Valid {
			snap.FilesChanged = int(filesChanged.Int64)
		}
	}

	snap.LatestVerificationStatus, snap.LatestVerificationFreshness,
		snap.LatestVerificationCommand, snap.LatestVerificationScope,
		snap.LatestVerificationAt = latestVerification(db)

	snap.BlockedReason = blockedReason(db)
	if snap.BlockedReason != "" {
		snap.WorkStatus = "blocked"
		snap.StatusReason = snap.BlockedReason
	}

	snap.AttentionItems = buildAttention(snap)
	snap.AttentionScore = attentionScore(snap)
	if snap.AttentionScore >= 25 && snap.WorkStatus == "idle" {
		snap.WorkStatus = "attention"
	}
	if snap.GitDirty && snap.WorkStatus == "idle" {
		snap.StatusReason = "dirty worktree"
	}
	return snap
}

func latestVerification(db *sql.DB) (status, freshness, command, scope, finishedAt string) {
	var id string
	err := db.QueryRow(`
		SELECT id FROM verification_runs
		WHERE finished_at IS NOT NULL
		ORDER BY finished_at DESC LIMIT 1`).Scan(&id)
	if err != nil {
		return "", "", "", "", ""
	}
	run, err := verification.GetRun(db, id)
	if err != nil || run == nil {
		return "", "", "", "", ""
	}
	status = run.Status
	command = run.Command
	scope = run.Scope
	finishedAt = run.FinishedAt
	if run.Status == "passed" {
		fresh, reason := verification.EvaluateFreshness(db, id)
		if fresh {
			freshness = "fresh"
		} else {
			freshness = reason
		}
	} else if run.Status == "failed" {
		freshness = "failed"
	}
	return status, freshness, command, scope, finishedAt
}

func blockedReason(db *sql.DB) string {
	item, err := planning.InFlight(db)
	if err != nil || item == nil {
		return ""
	}
	var note string
	err = db.QueryRow(`
		SELECT note FROM status_log
		WHERE plan_item_id=? AND status='in_progress'
		ORDER BY created_at DESC LIMIT 1`, item.ID).Scan(&note)
	if err != nil || note == "" {
		return ""
	}
	lower := strings.ToLower(note)
	for _, pat := range []string{"blocked", "blocker", "waiting", "stuck", "cannot proceed", "can't proceed"} {
		if strings.Contains(lower, pat) {
			return note
		}
	}
	return ""
}

func buildAttention(s Snapshot) []AttentionItem {
	var items []AttentionItem
	if s.Error != "" {
		return s.AttentionItems
	}
	if s.OpenHighFeedback > 0 {
		items = append(items, AttentionItem{
			Kind: "high_feedback", Severity: "high",
			Title: "open high/critical feedback", Status: "open",
		})
	}
	if s.OpenHighFindings > 0 {
		items = append(items, AttentionItem{
			Kind: "high_findings", Severity: "high",
			Title: "open high/critical review findings", Status: "open",
		})
	}
	if s.StaleArchNotes > 0 {
		items = append(items, AttentionItem{
			Kind: "stale_arch", Severity: "medium",
			Title: "stale architecture notes", Status: "open",
			Target: "arch list --stale",
		})
	}
	if s.MissedCalls7d >= 10 {
		items = append(items, AttentionItem{
			Kind: "missed_cli", Severity: "medium",
			Title: "elevated missed CLI calls (7d)", Status: "open",
		})
	}
	if s.LatestVerificationFreshness != "" && s.LatestVerificationFreshness != "fresh" && s.LatestVerificationStatus == "passed" {
		items = append(items, AttentionItem{
			Kind: "verification_stale", Severity: "high",
			Title: "verification no longer fresh: " + s.LatestVerificationFreshness, Status: "open",
		})
	}
	if s.GitDirty && s.LatestVerificationFreshness != "fresh" {
		items = append(items, AttentionItem{
			Kind: "dirty_unverified", Severity: "medium",
			Title: "dirty worktree without fresh verification", Status: "open",
		})
	}
	if s.BlockedReason != "" {
		items = append(items, AttentionItem{
			Kind: "blocked_work", Severity: "high",
			Title: s.BlockedReason, Status: "open",
		})
	}
	if s.OverdueReminders > 0 {
		items = append(items, AttentionItem{
			Kind: "overdue_reminder", Severity: "medium",
			Title: "overdue reminders", Status: "open",
		})
	}
	if s.InProgressItems > 0 && s.InFlightTitle != "" {
		items = append(items, AttentionItem{
			Kind: "in_progress", Severity: "low",
			Title: s.InFlightTitle, Status: "in_progress",
		})
	}
	return items
}

func attentionScore(s Snapshot) int {
	if s.SyncStatus == "missing" || s.SyncStatus == "error" {
		return s.AttentionScore
	}
	score := 0
	score += s.OpenHighFeedback * 15
	score += s.OpenHighFindings * 12
	score += s.StaleArchNotes * 3
	if s.LatestVerificationFreshness != "" && s.LatestVerificationFreshness != "fresh" && s.LatestVerificationStatus == "passed" {
		score += 20
	}
	if s.GitDirty {
		score += 5
	}
	if s.BlockedReason != "" {
		score += 30
	}
	if s.MissedCalls7d >= 10 {
		score += 8
	}
	if s.OverdueReminders > 0 {
		score += 5
	}
	return score
}

func encodeSnapshot(s Snapshot) (string, error) {
	b, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func decodeSnapshot(raw string) (Snapshot, error) {
	var s Snapshot
	if raw == "" {
		return s, nil
	}
	err := json.Unmarshal([]byte(raw), &s)
	return s, err
}
