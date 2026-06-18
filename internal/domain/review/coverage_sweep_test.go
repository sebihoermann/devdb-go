package review

import (
	"database/sql"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestScanFindingWithLines(t *testing.T) {
	db, _ := testutil.TempDB(t)
	runID, _ := StartRun(db, []string{"."}, "default", "", "test")
	line := 3
	end := 5
	findingID, _ := AddFinding(db, runID, FindingInput{
		FilePath: "b.go", LineStart: &line, LineEnd: &end,
		Principle: "dry", Title: "dup", Recommendation: "extract",
		Severity: "high", Confidence: "high", Effort: "small",
	}, "test")
	f, err := GetFinding(db, findingID)
	if err != nil || f == nil || f.LineStart == nil {
		t.Fatalf("finding=%+v err=%v", f, err)
	}
}

func TestResolveRunIDFullNonMatch(t *testing.T) {
	db, _ := testutil.TempDB(t)
	runID, _ := StartRun(db, []string{"."}, "default", "", "test")
	fake := "00000000000000000000000000000000"
	if fake == runID {
		t.Skip("collision")
	}
	_, err := resolveRunID(db, fake)
	if err == nil {
		t.Fatal("expected no match")
	}
}

func TestListFindingsAllFilters(t *testing.T) {
	db, _ := testutil.TempDB(t)
	runID, _ := StartRun(db, []string{"pkg"}, "extended", "", "test")
	_, _ = AddFinding(db, runID, FindingInput{
		FilePath: "pkg/a.go", Principle: "dead-code", Title: "unused",
		Recommendation: "remove", Severity: "medium", Confidence: "medium", Effort: "med",
	}, "test")
	rows, err := ListFindings(db, ListFilter{
		RunID: runID, Status: "open", Principle: "dead-code",
		FilePath: "pkg/a.go", Severity: "med", Limit: 10,
	})
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%d err=%v", len(rows), err)
	}
}

func TestRenderReportWithFindings(t *testing.T) {
	db, _ := testutil.TempDB(t)
	runID, _ := StartRun(db, []string{"."}, "default", "", "test")
	_, _ = AddFinding(db, runID, FindingInput{
		Principle: "kiss", Title: "complex", Recommendation: "simplify",
		Severity: "low", Confidence: "low", Effort: "trivial",
	}, "test")
	_, _ = FinishRun(db, runID, "summary")
	text, err := RenderReport(db, runID[:8])
	if err != nil || len(text) < 50 {
		t.Fatalf("report len=%d err=%v", len(text), err)
	}
}

func TestPriorityValueBranches(t *testing.T) {
	f := Finding{Severity: "high", Confidence: "high", Effort: "trivial"}
	if priorityValue(f) <= priorityValue(Finding{Severity: "low", Confidence: "low", Effort: "large"}) {
		t.Fatal("high should outrank low")
	}
}

// ensure sql import used for scanFinding via GetFinding path
var _ = sql.ErrNoRows
