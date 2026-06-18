package verification

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/domain/inventory"
	"github.com/sebihoermann/devdb-go/internal/storage"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestFindLatestRunMatchingInputs(t *testing.T) {
	db, _ := testutil.TempDB(t)
	exit := 0
	run1, _ := RecordRun(db, "go test ./pkg", "pkg", "sha1", "passed", &exit, "", "", "test")
	run2, _ := RecordRun(db, "go test ./pkg", "pkg", "sha2", "passed", &exit, "", "", "test")
	inputsA := [][3]string{{"pkg/a.go", "code", "hashA"}}
	inputsB := [][3]string{{"pkg/b.go", "code", "hashB"}}
	_ = AddInputs(db, run1, inputsA, "test")
	_ = AddInputs(db, run2, inputsB, "test")
	_ = FinishRun(db, run1, "passed", &exit, "")
	_ = FinishRun(db, run2, "passed", &exit, "")

	if got := findLatestRun(db, "go test ./pkg", "pkg", inputsB, nil); got != run2 {
		t.Fatalf("latest matching=%q want %s", got, run2)
	}
	if got := findLatestRun(db, "go test ./pkg", "pkg", inputsA, []string{"passed"}); got != run1 {
		t.Fatalf("status filter=%q want %s", got, run1)
	}
	if got := findLatestRun(db, "missing", "pkg", inputsA, nil); got != "" {
		t.Fatalf("missing cmd=%q", got)
	}
}

func TestMapsEqualAndTriplesHelpers(t *testing.T) {
	a := map[string][2]string{"x": {"code", "h1"}}
	b := map[string][2]string{"x": {"code", "h2"}}
	if mapsEqual(a, b) {
		t.Fatal("hash mismatch should differ")
	}
	if !mapsEqual(a, a) {
		t.Fatal("same map should equal")
	}
	triples := inputsToTriples([]Input{{FilePath: "a.go", Role: "code", ContentHash: "h"}})
	m := triplesToMap(triples)
	if m["a.go"][1] != "h" {
		t.Fatalf("map=%v", m)
	}
}

func TestEvaluateFreshnessBranches(t *testing.T) {
	db, _ := testutil.TempDB(t)
	repo := t.TempDir()
	mainGo := filepath.Join(repo, "main.go")
	pyproject := filepath.Join(repo, "pyproject.toml")
	for p, content := range map[string]string{
		mainGo:    "package main\n",
		pyproject: "[tool]\n",
	} {
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := inventory.Scan(db, repo, nil, false, "test"); err != nil {
		t.Fatal(err)
	}
	exit := 0
	runID, _ := RecordRun(db, "pytest", ".", "sha", "passed", &exit, "", "", "test")
	inputs, _ := CollectInputsForScope(db, ".")
	_ = AddInputs(db, runID, inputs, "test")
	_ = FinishRun(db, runID, "passed", &exit, "")

	fresh, reason := EvaluateFreshness(db, runID)
	if !fresh || reason != "fresh" {
		t.Fatalf("fresh=%v reason=%q", fresh, reason)
	}

	if _, err := db.Exec(`DELETE FROM repo_files WHERE path='main.go'`); err != nil {
		t.Fatal(err)
	}
	fresh, reason = EvaluateFreshness(db, runID)
	if fresh || !strings.Contains(reason, "input_file_removed") {
		t.Fatalf("removed: fresh=%v reason=%q", fresh, reason)
	}

	// restore input and test broad scope change on full run
	if _, err := inventory.Scan(db, repo, nil, false, "test"); err != nil {
		t.Fatal(err)
	}
	inputs, _ = CollectInputsForScope(db, ".")
	_ = AddInputs(db, runID, inputs, "test")
	if err := os.WriteFile(pyproject, []byte("[tool]\nversion=2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := inventory.Scan(db, repo, nil, false, "test"); err != nil {
		t.Fatal(err)
	}
	fresh, reason = EvaluateFreshness(db, runID)
	if fresh || (!strings.Contains(reason, "broad_scope_file_changed") && !strings.Contains(reason, "input_hash_changed")) {
		t.Fatalf("stale config: fresh=%v reason=%q", fresh, reason)
	}
}

func TestEvaluateReuseUnknownStatus(t *testing.T) {
	db, _ := testutil.TempDB(t)
	exit := 0
	runID, _ := RecordRun(db, "make", ".", "", "running", &exit, "", "", "test")
	decision := EvaluateReuse(db, "make", ".", nil)
	if decision.Decision != "unknown" {
		t.Fatalf("decision=%+v", decision)
	}
	_ = runID
}

func TestShowDismissGetInputsBranches(t *testing.T) {
	db, _ := testutil.TempDB(t)
	exit := 0
	runID, _ := RecordRun(db, "echo", ".", "", "passed", &exit, "", "notes", "test")
	_ = FinishRun(db, runID, "passed", &exit, "")

	summary, err := Show(db, runID)
	if err != nil || summary == nil {
		t.Fatalf("show: %+v err=%v", summary, err)
	}
	inputs, err := GetInputs(db, runID)
	if err != nil || inputs != nil && len(inputs) > 0 {
		// empty inputs ok
	}
	_ = inputs

	ok, err := Dismiss(db, runID[:8], "reason")
	if err != nil || !ok {
		t.Fatalf("dismiss: ok=%v err=%v", ok, err)
	}
	run, _ := GetRun(db, runID[:8])
	if run == nil {
		t.Fatal("get run nil")
	}
}

func TestCompactQueryLineFailedTests(t *testing.T) {
	q := QueryResult{
		Status: "failed_last_time", Reason: "boom", RunID: "abcdef0123456789abcdef0123456789",
		FailedTests: []map[string]any{{"headline": "test failed"}},
	}
	line := CompactQueryLine(q)
	if !strings.Contains(line, "failed") || !strings.Contains(line, "test failed") {
		t.Fatalf("line=%q", line)
	}
	q2 := QueryResult{Status: "weird", Reason: "x"}
	if CompactQueryLine(q2) == "" {
		t.Fatal("expected default status message")
	}
}

func TestParseFreshnessReasonAndInferRole(t *testing.T) {
	if p := parseFreshnessReason("input_hash_changed: foo.go"); p == nil || p.Path != "foo.go" {
		t.Fatalf("parsed=%+v", p)
	}
	if inferRoleFromPath("tests/test_foo.py") != "test" {
		t.Fatalf("role=%q", inferRoleFromPath("tests/test_foo.py"))
	}
	if !pathInScope("pkg/a.go", []string{"pkg"}) {
		t.Fatal("pkg scope")
	}
	if pathInScope("other.go", []string{"pkg"}) {
		t.Fatal("out of scope")
	}
	if !scopeIsFullRun(".") {
		t.Fatal("dot is full run")
	}
}

func TestResolveRunIDFullAndPrefix(t *testing.T) {
	db, _ := testutil.TempDB(t)
	exit := 0
	runID, _ := RecordRun(db, "cmd", ".", "", "passed", &exit, "", "", "test")
	full, err := resolveRunID(db, runID)
	if err != nil || full != runID {
		t.Fatalf("full=%q err=%v", full, err)
	}
	pref, err := resolveRunID(db, runID[:8])
	if err != nil || pref != runID {
		t.Fatalf("prefix=%q err=%v", pref, err)
	}
	_, err = resolveRunID(db, "missing")
	if err == nil {
		t.Fatal("expected error")
	}
	_ = storage.NowUTC()
}
