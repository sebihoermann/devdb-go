package status

import (
	"database/sql"

	"github.com/sebihoermann/devdb-go/internal/domain/architecture"
	"github.com/sebihoermann/devdb-go/internal/domain/planning"
	"github.com/sebihoermann/devdb-go/internal/git"
)

// Snapshot is compact delivery state.
type Snapshot struct {
	Overall       string         `json:"overall"`
	InFlight      *planning.PlanItem `json:"in_flight,omitempty"`
	OpenItems     int            `json:"open_plan_items"`
	InProgress    int            `json:"in_progress_plan_items"`
	OpenFeedback  int            `json:"open_feedback"`
	GitBranch     string         `json:"git_branch,omitempty"`
	GitDirty      bool           `json:"git_dirty"`
	GitAhead      int            `json:"git_ahead"`
	GitBehind     int            `json:"git_behind"`
	SchemaKind    string         `json:"schema_kind"`
	SchemaVersion int            `json:"schema_version"`
}

// Build assembles a status snapshot.
func Build(db *sql.DB, repoRoot string, kind string, version int) (Snapshot, error) {
	s := Snapshot{SchemaKind: kind, SchemaVersion: version}
	s.GitBranch = git.Branch(repoRoot)
	s.GitDirty = git.IsDirty(repoRoot)

	_ = db.QueryRow(`SELECT COUNT(*) FROM plan_items WHERE status IN ('planned','in_progress')`).Scan(&s.OpenItems)
	_ = db.QueryRow(`SELECT COUNT(*) FROM plan_items WHERE status='in_progress'`).Scan(&s.InProgress)
	_ = db.QueryRow(`SELECT COUNT(*) FROM feedback WHERE status='open'`).Scan(&s.OpenFeedback)

	item, err := planning.InFlight(db)
	if err != nil {
		return s, err
	}
	s.InFlight = item

	switch {
	case s.InProgress > 0:
		s.Overall = "active"
	case s.OpenItems > 0:
		s.Overall = "planned"
	default:
		s.Overall = "idle"
	}
	if s.GitDirty {
		s.Overall += " · dirty worktree"
	}
	return s, nil
}

// QualitySignals are trust indicators.
type QualitySignals struct {
	OpenHighFeedback int `json:"open_high_feedback"`
	StaleArchNotes   int `json:"stale_arch_notes"`
	OpenFindings     int `json:"open_findings"`
	MissedCalls7d    int `json:"missed_calls_7d"`
	OpenTasks        int `json:"open_tasks"`
	OpenReminders    int `json:"open_reminders"`
}

// Quality builds trust snapshot.
func Quality(db *sql.DB) (QualitySignals, error) {
	var q QualitySignals
	_ = db.QueryRow(`
		SELECT COUNT(*) FROM feedback
		WHERE status='open' AND severity IN ('high','critical')`).Scan(&q.OpenHighFeedback)
	_ = db.QueryRow(`
		SELECT COUNT(*) FROM review_findings WHERE status='open'`).Scan(&q.OpenFindings)
	_ = db.QueryRow(`
		SELECT COUNT(*) FROM missed_cli_calls
		WHERE created_at >= datetime('now', '-7 days')`).Scan(&q.MissedCalls7d)
	_ = db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE status='open'`).Scan(&q.OpenTasks)
	_ = db.QueryRow(`SELECT COUNT(*) FROM reminders WHERE status='open'`).Scan(&q.OpenReminders)
	q.StaleArchNotes, _ = architecture.CountStale(db)
	return q, nil
}

// VerboseSnapshot adds diagnostic detail for --verbose reads.
type VerboseSnapshot struct {
	Snapshot
	Quality QualitySignals `json:"quality"`
}
