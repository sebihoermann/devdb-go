package grasscutter

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/sebihoermann/devdb-go/internal/domain/review"
)

const grassCutterFileCap = 50

// RunResult is the outcome of a grass-cutter pass.
type RunResult struct {
	RunID       string              `json:"run_id,omitempty"`
	Candidates  []Candidate         `json:"candidates"`
	Counts      map[string]int      `json:"counts"`
	Summary     string              `json:"summary"`
	Persisted   bool                `json:"persisted"`
	PersistedN  int                 `json:"persisted_count,omitempty"`
}

// Run discovers heuristic candidates and optionally opens a grass-cutter review run.
func Run(db *sql.DB, repoRoot string, scopePaths, principles []string, dryRun bool, gitSHA, modelID string) (RunResult, error) {
	candidates, counts, err := Discover(repoRoot, db, scopePaths, principles)
	if err != nil {
		return RunResult{}, err
	}
	summary := formatSummary(candidates, counts)
	result := RunResult{
		Candidates: candidates,
		Counts:     counts,
		Summary:    summary,
	}
	if dryRun {
		result.Persisted = false
		return result, nil
	}

	toPersist := capCandidates(candidates, grassCutterFileCap)
	if len(toPersist) < len(candidates) {
		summary += fmt.Sprintf(" (persisted %d/%d after per-file cap %d)", len(toPersist), len(candidates), grassCutterFileCap)
		result.Summary = summary
	}

	if len(scopePaths) == 0 {
		scopePaths = []string{"."}
	}
	runID, err := review.StartRun(db, scopePaths, "grass-cutter", gitSHA, modelID)
	if err != nil {
		return RunResult{}, err
	}
	result.RunID = runID
	result.Persisted = true

	for _, c := range toPersist {
		in := review.FindingInput{
			FilePath:       c.FilePath,
			LineStart:      c.LineStart,
			LineEnd:        c.LineEnd,
			Principle:      c.Principle,
			Title:          c.Title,
			Recommendation: c.Recommendation,
			Severity:       c.Severity,
			Confidence:     c.Confidence,
			Effort:         c.Effort,
		}
		if _, err := review.AddFinding(db, runID, in, modelID); errors.Is(err, review.ErrCapExceeded) {
			continue
		} else if err != nil {
			_, _ = review.FinishRun(db, runID, summary)
			return RunResult{}, err
		}
		result.PersistedN++
	}

	if _, err := review.FinishRun(db, runID, summary); err != nil && !errors.Is(err, review.ErrRunFinished) {
		return RunResult{}, err
	}
	return result, nil
}
