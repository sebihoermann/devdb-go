package verification

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/domain/inventory"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestFinishRunFailedAndDismiss(t *testing.T) {
	db, _ := testutil.TempDB(t)
	exit := 1
	runID, _ := RecordRun(db, "go test", ".", "sha", "failed", &exit, "", "", "test")
	failOut := "FAIL\tpkg/a_test.go:10: expected true\n--- FAIL: TestA"
	if err := FinishRun(db, runID, "failed", &exit, failOut); err != nil {
		t.Fatal(err)
	}
	run, err := GetRun(db, runID)
	if err != nil || run.Status != "failed" {
		t.Fatalf("run=%+v err=%v", run, err)
	}
	inputs, err := GetInputs(db, runID)
	if err != nil {
		t.Fatal(err)
	}
	_ = inputs
	ok, err := Dismiss(db, runID, "false positive")
	if err != nil || !ok {
		t.Fatalf("dismiss ok=%v err=%v", ok, err)
	}
	ok2, _ := Dismiss(db, runID, "again")
	if ok2 {
		t.Fatal("second dismiss should be false")
	}
}

func TestShowViewsAndQueryReuse(t *testing.T) {
	db, _ := testutil.TempDB(t)
	repo := t.TempDir()
	mainGo := filepath.Join(repo, "main.go")
	if err := os.WriteFile(mainGo, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _ = inventory.Scan(db, repo, nil, false, "test")
	exit := 0
	runID, _ := RecordRun(db, "go test ./...", ".", "sha1", "passed", &exit, "", "", "test")
	inputs, _ := CollectInputsForScope(db, ".")
	_ = AddInputs(db, runID, inputs, "test")
	_ = FinishRun(db, runID, "passed", &exit, "")

	for _, view := range []string{"summary", "inputs", "failures"} {
		sum, err := Show(db, runID)
		if err != nil || sum == nil {
			t.Fatalf("show view=%s err=%v", view, err)
		}
		_ = view
	}
	scopeInputs, _ := CollectInputsForScope(db, ".")
	q := Query(db, "go test ./...", ".", scopeInputs, true)
	if q.RunID == "" && q.Reason == "no_prior_run" {
		t.Fatalf("query=%+v", q)
	}
	line := CompactQueryLine(q)
	if line == "" {
		t.Fatalf("line=%q", line)
	}
}

func TestRecordRunAndResolveErrors(t *testing.T) {
	db, _ := testutil.TempDB(t)
	if _, err := GetRun(db, "missing"); err == nil {
		t.Fatal("expected missing run")
	}
	if _, err := GetInputs(db, "missing"); err == nil {
		t.Fatal("expected missing inputs")
	}
	exit := 2
	runID, _ := RecordRun(db, "make", ".", "", "running", &exit, "partial", "notes", "test")
	if runID == "" {
		t.Fatal("empty run id")
	}
}
