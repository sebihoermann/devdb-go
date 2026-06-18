package review

import (
	"errors"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestNormalizerDefaults(t *testing.T) {
	if NormalizeSeverity("bogus") != "bogus" {
		t.Fatal("severity default")
	}
	if NormalizeConfidence("bogus") != "bogus" {
		t.Fatal("confidence default")
	}
	if NormalizeEffort("bogus") != "bogus" {
		t.Fatal("effort default")
	}
}

func TestValidateFindingAllInvalid(t *testing.T) {
	in := FindingInput{
		Principle: "kiss", Title: "t", Recommendation: "r",
		Severity: "bogus", Confidence: "high", Effort: "small",
	}
	if err := validateFinding("default", in); err == nil {
		t.Fatal("bad severity")
	}
	in.Severity = "low"
	in.Confidence = "bogus"
	if err := validateFinding("default", in); err == nil {
		t.Fatal("bad confidence")
	}
	in.Confidence = "high"
	in.Effort = "bogus"
	if err := validateFinding("default", in); err == nil {
		t.Fatal("bad effort")
	}
}

func TestPriorityValueUnknownWeights(t *testing.T) {
	f := Finding{Severity: "unknown", Confidence: "unknown", Effort: "unknown"}
	if priorityValue(f) <= 0 {
		t.Fatalf("priority=%v", priorityValue(f))
	}
}

func TestImportFindingsErrorPaths(t *testing.T) {
	db, _ := testutil.TempDB(t)
	runID, _ := StartRun(db, []string{"."}, "default", "", "test")
	bad := FindingInput{
		Principle: "not-real", Title: "x", Recommendation: "y",
		Severity: "low", Confidence: "low", Effort: "trivial",
	}
	result, err := ImportFindings(db, runID, []FindingInput{bad}, false, "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Errors) != 1 {
		t.Fatalf("errors=%d", len(result.Errors))
	}
	_, _ = FinishRun(db, runID, "done")
	_, err = ImportFindings(db, runID, []FindingInput{bad}, false, "test")
	if !errors.Is(err, ErrRunFinished) {
		t.Fatalf("finished run import: %v", err)
	}
}

func TestResolveRunAndFindingIDs(t *testing.T) {
	db, _ := testutil.TempDB(t)
	runID, _ := StartRun(db, []string{"."}, "default", "", "test")
	findingID, _ := AddFinding(db, runID, FindingInput{
		FilePath: "a.go", Principle: "kiss", Title: "t", Recommendation: "r",
		Severity: "low", Confidence: "low", Effort: "trivial",
	}, "test")
	got, err := resolveRunID(db, runID)
	if err != nil || got != runID {
		t.Fatalf("run=%q err=%v", got, err)
	}
	got, err = resolveFindingID(db, findingID[:8])
	if err != nil || got != findingID {
		t.Fatalf("finding=%q err=%v", got, err)
	}
}

func TestFinishRunMissingAndGetFinding(t *testing.T) {
	db, _ := testutil.TempDB(t)
	f, err := GetFinding(db, "zzzzzzzz")
	if err == nil || f != nil {
		t.Fatalf("get missing: %+v err=%v", f, err)
	}
	run, err := GetRun(db, "zzzzzzzz")
	if err == nil || run != nil {
		t.Fatalf("get missing run: %+v err=%v", run, err)
	}
}

func TestListFindingsSortAndFilters(t *testing.T) {
	db, _ := testutil.TempDB(t)
	runID, _ := StartRun(db, []string{"."}, "extended", "", "test")
	for _, pr := range []struct {
		file, principle, sev string
	}{
		{"a.go", "kiss", "high"},
		{"b.go", "dry", "low"},
	} {
		_, _ = AddFinding(db, runID, FindingInput{
			FilePath: pr.file, Principle: pr.principle, Title: pr.file,
			Recommendation: "fix", Severity: pr.sev, Confidence: "high", Effort: "small",
		}, "test")
	}
	rows, err := ListFindings(db, ListFilter{RunID: runID, Status: "open", Limit: 1})
	if err != nil || len(rows) != 1 {
		t.Fatalf("limit: %d err=%v", len(rows), err)
	}
	all, err := ListFindings(db, ListFilter{RunID: runID, Principle: "dry"})
	if err != nil || len(all) != 1 {
		t.Fatalf("principle filter: %d", len(all))
	}
}

func TestRenderReportAndCompactLinesEmpty(t *testing.T) {
	db, _ := testutil.TempDB(t)
	runID, _ := StartRun(db, []string{"."}, "default", "", "test")
	_, _ = FinishRun(db, runID, "empty run")
	text, err := RenderReport(db, runID)
	if err != nil || text == "" {
		t.Fatalf("report err=%v len=%d", err, len(text))
	}
	if len(CompactLines(nil)) != 0 {
		t.Fatal("nil compact")
	}
}

func TestAddFindingUncappedValidation(t *testing.T) {
	db, _ := testutil.TempDB(t)
	runID, _ := StartRun(db, []string{"."}, "default", "", "test")
	_, err := addFindingUncapped(db, runID, FindingInput{
		Principle: "bad", Title: "t", Recommendation: "r",
		Severity: "low", Confidence: "low", Effort: "trivial",
	}, "test")
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestPrinciplesDefaultTier(t *testing.T) {
	p := PrinciplesForTier("unknown-tier")
	if len(p) < 5 {
		t.Fatalf("default principles=%d", len(p))
	}
}
