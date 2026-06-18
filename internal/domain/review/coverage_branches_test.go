package review

import (
	"testing"

	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestScanFindingAndListFilters(t *testing.T) {
	db, _ := testutil.TempDB(t)
	runID, _ := StartRun(db, []string{"pkg"}, "extended", "", "test")
	findingID, _ := AddFinding(db, runID, FindingInput{
		FilePath: "pkg/a.go", Principle: "kiss", Title: "issue",
		Recommendation: "simplify", Severity: "high", Confidence: "high", Effort: "small",
	}, "test")
	open, err := ListFindings(db, ListFilter{Status: "open", RunID: runID, Limit: 10})
	if err != nil || len(open) == 0 {
		t.Fatalf("open=%d err=%v", len(open), err)
	}
	_, _ = ResolveFinding(db, findingID, "evidence", "resolved", "fixed")
	resolved, err := ListFindings(db, ListFilter{Status: "resolved", Principle: "kiss"})
	if err != nil || len(resolved) == 0 {
		t.Fatalf("resolved=%d err=%v", len(resolved), err)
	}
}

func TestRenderReportAndPrinciples(t *testing.T) {
	db, _ := testutil.TempDB(t)
	runID, _ := StartRun(db, []string{"."}, "default", "", "test")
	_, _ = AddFinding(db, runID, FindingInput{
		FilePath: "main.go", Principle: "dry", Title: "dup",
		Recommendation: "extract", Severity: "med", Confidence: "med", Effort: "medium",
	}, "test")
	_, _ = FinishRun(db, runID, "summary text")
	md, err := RenderReport(db, runID)
	if err != nil || md == "" {
		t.Fatalf("report len=%d err=%v", len(md), err)
	}
	tiers := PrinciplesForTier("extended")
	if len(tiers) == 0 {
		t.Fatal("no principles")
	}
}

func TestImportFindingsForceCap(t *testing.T) {
	db, _ := testutil.TempDB(t)
	runID, _ := StartRun(db, []string{"."}, "default", "", "test")
	inputs := []FindingInput{{
		FilePath: "b.go", Principle: "kiss", Title: "imported",
		Recommendation: "note", Severity: "low", Confidence: "low", Effort: "trivial",
	}}
	applied, err := ImportFindings(db, runID, inputs, true, "test")
	if err != nil || len(applied.Imported) != 1 {
		t.Fatalf("apply=%+v err=%v", applied, err)
	}
}
