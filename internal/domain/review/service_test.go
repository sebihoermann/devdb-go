package review_test

import (
	"errors"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/domain/review"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestReviewWorkflow(t *testing.T) {
	db, _ := testutil.TempDB(t)

	runID, err := review.StartRun(db, []string{"."}, "default", "abc123", "test")
	if err != nil {
		t.Fatal(err)
	}

	in := review.FindingInput{
		FilePath: "main.go", Principle: "kiss", Title: "too complex",
		Recommendation: "simplify", Severity: "med", Confidence: "high", Effort: "small",
	}
	findingID, err := review.AddFinding(db, runID, in, "test")
	if err != nil {
		t.Fatal(err)
	}

	findings, err := review.ListFindings(db, review.ListFilter{RunID: runID})
	if err != nil || len(findings) != 1 {
		t.Fatalf("list: %d findings err=%v", len(findings), err)
	}

	ok, err := review.FinishRun(db, runID, "one finding")
	if err != nil || !ok {
		t.Fatalf("finish: ok=%v err=%v", ok, err)
	}

	if _, err := review.AddFinding(db, runID, in, "test"); !errors.Is(err, review.ErrRunFinished) {
		t.Fatalf("expected finished error, got %v", err)
	}

	ok, err = review.ResolveFinding(db, findingID, "deadbeef", "resolved", "")
	if err != nil || !ok {
		t.Fatalf("resolve: ok=%v err=%v", ok, err)
	}

	text, err := review.RenderReport(db, runID)
	if err != nil || text == "" {
		t.Fatalf("report: err=%v len=%d", err, len(text))
	}
}

func TestFindingCap(t *testing.T) {
	db, _ := testutil.TempDB(t)
	runID, _ := review.StartRun(db, []string{"."}, "default", "", "test")
	in := review.FindingInput{
		FilePath: "a.go", Principle: "dry", Title: "dup", Recommendation: "extract",
		Severity: "low", Confidence: "medium", Effort: "trivial",
	}
	for i := 0; i < 3; i++ {
		if _, err := review.AddFinding(db, runID, in, "test"); err != nil {
			t.Fatalf("finding %d: %v", i, err)
		}
	}
	if _, err := review.AddFinding(db, runID, in, "test"); !errors.Is(err, review.ErrCapExceeded) {
		t.Fatalf("expected cap error, got %v", err)
	}
}

func TestNormalizersAndPrinciples(t *testing.T) {
	if got := review.NormalizeSeverity("medium"); got != "med" {
		t.Fatalf("severity=%q", got)
	}
	if got := review.NormalizeConfidence("med"); got != "medium" {
		t.Fatalf("confidence=%q", got)
	}
	if got := review.NormalizeEffort("medium"); got != "med" {
		t.Fatalf("effort=%q", got)
	}
	principles := review.PrinciplesForTier("extended")
	if len(principles) < 10 {
		t.Fatalf("extended principles=%d", len(principles))
	}
	grass := review.PrinciplesForTier("grass-cutter")
	if len(grass) < 10 {
		t.Fatalf("grass principles=%d", len(grass))
	}
}

func TestImportFindingsForceCap(t *testing.T) {
	db, _ := testutil.TempDB(t)
	runID, _ := review.StartRun(db, []string{"pkg"}, "default", "", "test")
	in := review.FindingInput{
		FilePath: "pkg/a.go", Principle: "kiss", Title: "t1", Recommendation: "fix",
		Severity: "low", Confidence: "low", Effort: "trivial",
	}
	for i := 0; i < 3; i++ {
		if _, err := review.AddFinding(db, runID, in, "test"); err != nil {
			t.Fatal(err)
		}
	}
	result, err := review.ImportFindings(db, runID, []review.FindingInput{in, in}, true, "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Imported) != 2 {
		t.Fatalf("imported=%d skipped=%d errors=%d", len(result.Imported), len(result.SkippedCap), len(result.Errors))
	}
}

func TestGetFindingAndCompactLines(t *testing.T) {
	db, _ := testutil.TempDB(t)
	runID, _ := review.StartRun(db, []string{"."}, "default", "", "test")
	line := 5
	findingID, err := review.AddFinding(db, runID, review.FindingInput{
		FilePath: "b.go", LineStart: &line, Principle: "dry", Title: "dup",
		Recommendation: "extract", Severity: "high", Confidence: "high", Effort: "small",
	}, "test")
	if err != nil {
		t.Fatal(err)
	}
	f, err := review.GetFinding(db, findingID[:8])
	if err != nil || f == nil || f.Title != "dup" {
		t.Fatalf("get finding: %+v err=%v", f, err)
	}
	lines := review.CompactLines([]review.Finding{*f})
	if len(lines) != 1 || lines[0] == "" {
		t.Fatalf("compact lines: %v", lines)
	}
}

func TestInvalidFindingValidation(t *testing.T) {
	db, _ := testutil.TempDB(t)
	runID, _ := review.StartRun(db, []string{"."}, "default", "", "test")
	_, err := review.AddFinding(db, runID, review.FindingInput{
		Principle: "not-a-principle", Title: "x", Recommendation: "y",
		Severity: "high", Confidence: "high", Effort: "small",
	}, "test")
	if err == nil {
		t.Fatal("expected invalid principle error")
	}
}

func TestImportFindingsSkippedCap(t *testing.T) {
	db, _ := testutil.TempDB(t)
	runID, _ := review.StartRun(db, []string{"."}, "default", "", "test")
	in := review.FindingInput{
		FilePath: "pkg/a.go", Principle: "kiss", Title: "t1", Recommendation: "fix",
		Severity: "low", Confidence: "low", Effort: "trivial",
	}
	for i := 0; i < 3; i++ {
		if _, err := review.AddFinding(db, runID, in, "test"); err != nil {
			t.Fatal(err)
		}
	}
	result, err := review.ImportFindings(db, runID, []review.FindingInput{in}, false, "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.SkippedCap) != 1 {
		t.Fatalf("skipped=%d", len(result.SkippedCap))
	}
}

func TestExtendedTierPrinciple(t *testing.T) {
	db, _ := testutil.TempDB(t)
	runID, _ := review.StartRun(db, []string{"."}, "extended", "", "test")
	if _, err := review.AddFinding(db, runID, review.FindingInput{
		FilePath: "a.go", Principle: "dead-code", Title: "unused", Recommendation: "remove",
		Severity: "low", Confidence: "high", Effort: "trivial",
	}, "test"); err != nil {
		t.Fatal(err)
	}
}

func TestResolveFindingRequiresEvidence(t *testing.T) {
	db, _ := testutil.TempDB(t)
	runID, _ := review.StartRun(db, []string{"."}, "default", "", "test")
	findingID, _ := review.AddFinding(db, runID, review.FindingInput{
		FilePath: "c.go", Principle: "kiss", Title: "t", Recommendation: "r",
		Severity: "low", Confidence: "low", Effort: "trivial",
	}, "test")
	if _, err := review.ResolveFinding(db, findingID, "", "resolved", ""); err == nil {
		t.Fatal("expected evidence required error")
	}
}

func TestResolveFindingByFullID(t *testing.T) {
	db, _ := testutil.TempDB(t)
	runID, _ := review.StartRun(db, []string{"."}, "default", "", "test")
	findingID, _ := review.AddFinding(db, runID, review.FindingInput{
		FilePath: "d.go", Principle: "dry", Title: "dup", Recommendation: "r",
		Severity: "low", Confidence: "low", Effort: "trivial",
	}, "test")
	ok, err := review.ResolveFinding(db, findingID, "", "wontfix", "declined")
	if err != nil || !ok {
		t.Fatalf("resolve full id: ok=%v err=%v", ok, err)
	}
}

func TestListFindingsFilters(t *testing.T) {
	db, _ := testutil.TempDB(t)
	runID, _ := review.StartRun(db, []string{"."}, "default", "", "test")
	if _, err := review.AddFinding(db, runID, review.FindingInput{
		FilePath: "d.go", Principle: "dry", Title: "dup", Recommendation: "r",
		Severity: "medium", Confidence: "medium", Effort: "med",
	}, "test"); err != nil {
		t.Fatal(err)
	}
	rows, err := review.ListFindings(db, review.ListFilter{
		Status: "open", RunID: runID, Principle: "dry", FilePath: "d.go", Severity: "med", Limit: 1,
	})
	if err != nil || len(rows) != 1 {
		t.Fatalf("filtered list: %d err=%v", len(rows), err)
	}
}

func TestScopedStartRun(t *testing.T) {
	db, _ := testutil.TempDB(t)
	if _, err := db.Exec(`INSERT INTO repo_files(path, kind, content_hash, last_seen_at) VALUES ('pkg/a.go','code','h',datetime('now')), ('other/b.go','code','h',datetime('now'))`); err != nil {
		t.Fatal(err)
	}
	runID, err := review.StartRun(db, []string{"pkg"}, "default", "", "test")
	if err != nil {
		t.Fatal(err)
	}
	run, err := review.GetRun(db, runID)
	if err != nil || run == nil || run.FilesTotal == nil || *run.FilesTotal != 1 {
		t.Fatalf("scoped run: %+v err=%v", run, err)
	}
}

func TestFinishRunNotFound(t *testing.T) {
	db, _ := testutil.TempDB(t)
	_, err := review.FinishRun(db, "missing", "")
	if err == nil {
		t.Fatal("expected error for missing run prefix")
	}
}
