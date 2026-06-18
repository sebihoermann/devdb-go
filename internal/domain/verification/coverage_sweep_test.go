package verification

import (
	"testing"

	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestFindLatestRunInputMismatch(t *testing.T) {
	db, _ := testutil.TempDB(t)
	exit := 0
	runID, _ := RecordRun(db, "make test", ".", "", "passed", &exit, "", "", "test")
	_ = AddInputs(db, runID, [][3]string{{"a.go", "code", "h1"}}, "test")
	_ = FinishRun(db, runID, "passed", &exit, "")
	if got := findLatestRun(db, "make test", ".", [][3]string{{"b.go", "code", "h2"}}, nil); got != "" {
		t.Fatalf("mismatch should not match, got %q", got)
	}
}

func TestParsePytestAndGenericFailures(t *testing.T) {
	db, _ := testutil.TempDB(t)
	exit := 1
	pyOut := "=========================== short test summary info ============================\nFAILED tests/a.py::test_b - assert 0\nERROR tests/c.py::test_d - boom\n"
	runID, _ := RecordRun(db, "pytest", "tests/", "", "failed", &exit, pyOut, "", "test")
	_ = replaceFailures(db, runID, pyOut)
	failures, _ := GetFailures(db, runID, 10)
	if len(failures) < 2 {
		t.Fatalf("pytest failures=%d", len(failures))
	}
	genID, _ := RecordRun(db, "cargo test", ".", "", "failed", &exit, "error: build failed\n", "", "test")
	_ = replaceFailures(db, genID, "error: build failed\n")
	gen, _ := GetFailures(db, genID, 5)
	if len(gen) != 1 || gen[0].FailureKind != "generic" {
		t.Fatalf("generic=%+v", gen)
	}
}

func TestEvaluateReuseMatchingRunMissing(t *testing.T) {
	db, _ := testutil.TempDB(t)
	decision := EvaluateReuse(db, "cmd", ".", [][3]string{{"x", "code", "h"}})
	if decision.Decision != "unknown" || decision.Reason != "no_prior_run" {
		t.Fatalf("decision=%+v", decision)
	}
}

func TestAddInputsAndDismissBranches(t *testing.T) {
	db, _ := testutil.TempDB(t)
	exit := 0
	runID, _ := RecordRun(db, "echo", ".", "", "passed", &exit, "", "", "test")
	if err := AddInputs(db, runID, nil, "test"); err != nil {
		t.Fatal(err)
	}
	_ = FinishRun(db, runID, "passed", &exit, "")
	ok, _ := Dismiss(db, "missing", "reason")
	if ok {
		t.Fatal("dismiss missing")
	}
}

func TestCompactQueryLineAllBranches(t *testing.T) {
	for _, q := range []QueryResult{
		{Status: "fresh_pass", Reason: "ok", RunID: "abcdef0123456789abcdef0123456789"},
		{Status: "stale_pass", Reason: "stale", ChangedFiles: []ChangedFile{{Path: "a.go"}}},
		{Status: "failed_last_time", FailedTests: []map[string]any{{"test_id": "t1"}}},
		{Status: "weird"},
	} {
		if CompactQueryLine(q) == "" {
			t.Fatalf("empty line for %+v", q)
		}
	}
}
