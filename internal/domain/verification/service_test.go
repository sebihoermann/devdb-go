package verification_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/domain/inventory"
	"github.com/sebihoermann/devdb-go/internal/domain/verification"
	"github.com/sebihoermann/devdb-go/internal/storage"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestVerificationReuseFresh(t *testing.T) {
	db, _ := testutil.TempDB(t)
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := inventory.Scan(db, repo, nil, false, "test"); err != nil {
		t.Fatal(err)
	}

	exit := 0
	runID, err := verification.RecordRun(db, "go test ./...", ".", "sha1", "passed", &exit, "", "notes", "test")
	if err != nil {
		t.Fatal(err)
	}
	inputs, err := verification.CollectInputsForScope(db, ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs) == 0 {
		t.Fatal("expected inputs from scan")
	}
	if err := verification.AddInputs(db, runID, inputs, "test"); err != nil {
		t.Fatal(err)
	}
	if err := verification.FinishRun(db, runID, "passed", &exit, ""); err != nil {
		t.Fatal(err)
	}

	decision := verification.Query(db, "go test ./...", ".", inputs, true)
	if decision.Decision != "reusable" {
		t.Fatalf("expected reusable, got %+v", decision)
	}
	line := verification.CompactQueryLine(decision)
	if line == "" {
		t.Fatal("expected compact query line")
	}
}

func TestVerificationFailedPytestParsing(t *testing.T) {
	db, _ := testutil.TempDB(t)
	exit := 1
	output := "=========================== short test summary info ============================\nFAILED tests/test_foo.py::test_bar - AssertionError\n"
	runID, err := verification.RecordRun(db, "pytest tests/", "tests/", "sha1", "failed", &exit, output, "", "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := verification.FinishRun(db, runID, "failed", &exit, output); err != nil {
		t.Fatal(err)
	}
	failures, err := verification.GetFailures(db, runID, 0)
	if err != nil || len(failures) != 1 {
		t.Fatalf("failures: %d err=%v", len(failures), err)
	}
	if failures[0].TestID != "tests/test_foo.py::test_bar" {
		t.Fatalf("test_id=%q", failures[0].TestID)
	}
}

func TestShowDismissAndStaleReuse(t *testing.T) {
	db, _ := testutil.TempDB(t)
	repo := t.TempDir()
	mainGo := filepath.Join(repo, "main.go")
	if err := os.WriteFile(mainGo, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := inventory.Scan(db, repo, nil, false, "test"); err != nil {
		t.Fatal(err)
	}

	exit := 0
	runID, err := verification.RecordRun(db, "go test ./...", ".", "sha1", "passed", &exit, "", "", "test")
	if err != nil {
		t.Fatal(err)
	}
	inputs, _ := verification.CollectInputsForScope(db, ".")
	_ = verification.AddInputs(db, runID, inputs, "test")
	_ = verification.FinishRun(db, runID, "passed", &exit, "")

	summary, err := verification.Show(db, runID[:8])
	if err != nil || summary == nil || summary.Run.ID != runID {
		t.Fatalf("show: %+v err=%v", summary, err)
	}

	ok, err := verification.Dismiss(db, runID, "flaky")
	if err != nil || !ok {
		t.Fatalf("dismiss: ok=%v err=%v", ok, err)
	}
	ok, _ = verification.Dismiss(db, runID, "again")
	if ok {
		t.Fatal("second dismiss should be no-op")
	}

	// mutate stored input hash to trigger stale freshness
	if _, err := db.Exec(`UPDATE repo_files SET content_hash='changed' WHERE path='main.go'`); err != nil {
		t.Fatal(err)
	}
	fresh, reason := verification.EvaluateFreshness(db, runID)
	if fresh || !strings.Contains(reason, "input_hash_changed") {
		t.Fatalf("fresh=%v reason=%q", fresh, reason)
	}
	reuse := verification.EvaluateReuse(db, "go test ./...", ".", inputs)
	if reuse.Decision != "rerun_required" {
		t.Fatalf("expected rerun_required, got %+v", reuse)
	}
}

func TestGenericFailureParsing(t *testing.T) {
	db, _ := testutil.TempDB(t)
	exit := 1
	output := "build failed: something went wrong\n"
	runID, err := verification.RecordRun(db, "make test", ".", "", "failed", &exit, output, "", "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := verification.FinishRun(db, runID, "failed", &exit, output); err != nil {
		t.Fatal(err)
	}
	failures, err := verification.GetFailures(db, runID, 5)
	if err != nil || len(failures) != 1 {
		t.Fatalf("failures=%d err=%v", len(failures), err)
	}
	if failures[0].FailureKind != "generic" {
		t.Fatalf("kind=%q", failures[0].FailureKind)
	}
}

func TestEvaluateFreshnessNoInputs(t *testing.T) {
	db, _ := testutil.TempDB(t)
	exit := 0
	runID, _ := verification.RecordRun(db, "go test", ".", "", "passed", &exit, "", "", "test")
	_ = verification.FinishRun(db, runID, "passed", &exit, "")
	fresh, reason := verification.EvaluateFreshness(db, runID)
	if fresh || reason != "no_inputs_stored" {
		t.Fatalf("fresh=%v reason=%q", fresh, reason)
	}
}

func TestEvaluateFreshnessMissingRun(t *testing.T) {
	db, _ := testutil.TempDB(t)
	if _, err := db.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
			t.Fatal(err)
		}
	})
	if _, err := db.Exec(`INSERT INTO verification_inputs(id, run_id, file_path, role, content_hash, created_at, model_id)
		VALUES ('missing-input', 'missing-run', 'main.go', 'source', 'hash', datetime('now'), 'test')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO repo_files(path, kind, content_hash, last_seen_at)
		VALUES ('main.go', 'go', 'hash', datetime('now'))`); err != nil {
		t.Fatal(err)
	}
	fresh, reason := verification.EvaluateFreshness(db, "missing-run")
	if fresh || reason != "run_missing" {
		t.Fatalf("fresh=%v reason=%q", fresh, reason)
	}
}

func TestFailedLastTimeReuse(t *testing.T) {
	db, _ := testutil.TempDB(t)
	exit := 1
	runID, err := verification.RecordRun(db, "pytest", "tests/", "", "failed", &exit, "FAILED t - err", "", "test")
	if err != nil {
		t.Fatal(err)
	}
	_ = verification.FinishRun(db, runID, "failed", &exit, "FAILED t - err")
	decision := verification.EvaluateReuse(db, "pytest", "tests/", nil)
	if decision.Decision != "failed_last_time" {
		t.Fatalf("decision=%+v", decision)
	}
	q := verification.Query(db, "pytest", "tests/", nil, false)
	if q.Status != "failed_last_time" {
		t.Fatalf("query status=%q", q.Status)
	}
	line := verification.CompactQueryLine(q)
	if line == "" {
		t.Fatal("expected line for failed_last_time")
	}
}

func TestScopedInputsCollect(t *testing.T) {
	db, _ := testutil.TempDB(t)
	if _, err := db.Exec(`INSERT INTO repo_files(path, kind, content_hash, last_seen_at) VALUES
		('pkg/a.go','code','h1',datetime('now')), ('tests/conftest.py','test','h2',datetime('now')), ('pyproject.toml','config','h3',datetime('now'))`); err != nil {
		t.Fatal(err)
	}
	inputs, err := verification.CollectInputsForScope(db, "pkg")
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs) != 1 || inputs[0][0] != "pkg/a.go" {
		t.Fatalf("scoped inputs=%v", inputs)
	}
	full, err := verification.CollectInputsForScope(db, ".")
	if err != nil || len(full) < 3 {
		t.Fatalf("full inputs=%d err=%v", len(full), err)
	}
}

func TestInputsWithNullContentHash(t *testing.T) {
	db, _ := testutil.TempDB(t)
	if _, err := db.Exec(`INSERT INTO repo_files(path, kind, content_hash, last_seen_at)
		VALUES ('generated', 'other', NULL, datetime('now'))`); err != nil {
		t.Fatal(err)
	}
	inputs, err := verification.CollectInputsForScope(db, ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs) != 1 || inputs[0] != [3]string{"generated", "source", ""} {
		t.Fatalf("inputs=%v", inputs)
	}
	exit := 0
	runID, err := verification.RecordRun(db, "generate", ".", "", "passed", &exit, "", "", "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := verification.AddInputs(db, runID, inputs, "test"); err != nil {
		t.Fatal(err)
	}
	if fresh, reason := verification.EvaluateFreshness(db, runID); !fresh {
		t.Fatalf("fresh=%v reason=%s", fresh, reason)
	}
}

func TestFileChangeEventsTriggerRerun(t *testing.T) {
	db, _ := testutil.TempDB(t)
	repo := t.TempDir()
	mainGo := filepath.Join(repo, "lib.go")
	if err := os.WriteFile(mainGo, []byte("package lib\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := inventory.Scan(db, repo, nil, false, "test"); err != nil {
		t.Fatal(err)
	}
	exit := 0
	runID, _ := verification.RecordRun(db, "go test ./lib", "lib", "", "passed", &exit, "", "", "test")
	inputs, _ := verification.CollectInputsForScope(db, "lib")
	_ = verification.AddInputs(db, runID, inputs, "test")
	_ = verification.FinishRun(db, runID, "passed", &exit, "")

	if err := os.WriteFile(mainGo, []byte("package lib\n\nfunc F() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := inventory.Scan(db, repo, nil, false, "test"); err != nil {
		t.Fatal(err)
	}
	reuse := verification.EvaluateReuse(db, "go test ./lib", "lib", inputs)
	if reuse.Decision != "rerun_required" {
		t.Fatalf("decision=%+v", reuse)
	}
	q := verification.Query(db, "go test ./lib", "lib", inputs, false)
	if q.Status != "stale_pass" {
		t.Fatalf("status=%q reason=%q", q.Status, q.Reason)
	}
	line := verification.CompactQueryLine(q)
	if !strings.Contains(line, "changed:") {
		t.Fatalf("line=%q", line)
	}
}

func TestGetRunByFullID(t *testing.T) {
	db, _ := testutil.TempDB(t)
	exit := 0
	runID, err := verification.RecordRun(db, "echo ok", ".", "", "passed", &exit, "", "", "test")
	if err != nil {
		t.Fatal(err)
	}
	run, err := verification.GetRun(db, runID)
	if err != nil || run == nil {
		t.Fatalf("get run: %+v err=%v", run, err)
	}
	_ = storage.NowUTC()
}
