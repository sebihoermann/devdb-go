package verification

import (
	"testing"

	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestRecordFinishDismissAndQuery(t *testing.T) {
	db, _ := testutil.TempDB(t)
	exit := 0
	runID, err := RecordRun(db, "go test ./...", ".", "sha1", "passed", &exit, "ok", "notes", "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := AddInputs(db, runID, [][3]string{{"main.go", "scope", "hash1"}}, "test"); err != nil {
		t.Fatal(err)
	}
	if err := FinishRun(db, runID, "passed", &exit, "ok"); err != nil {
		t.Fatal(err)
	}
	fresh, reason := EvaluateFreshness(db, runID)
	if !fresh && reason == "" {
		t.Fatal("expected freshness evaluation")
	}
	show, err := Show(db, runID)
	if err != nil || show == nil {
		t.Fatalf("show=%+v err=%v", show, err)
	}
	inputs, err := GetInputs(db, runID)
	if err != nil || len(inputs) == 0 {
		t.Fatalf("inputs=%d err=%v", len(inputs), err)
	}
	dismissed, err := Dismiss(db, runID, "obsolete")
	if err != nil || !dismissed {
		t.Fatalf("dismissed=%v err=%v", dismissed, err)
	}
}

func TestRecordFailedRunWithFailures(t *testing.T) {
	db, _ := testutil.TempDB(t)
	exit := 1
	output := "FAIL pkg TestFoo\n--- FAIL: TestFoo\n"
	runID, err := RecordRun(db, "go test ./...", ".", "", "failed", &exit, output, "", "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := FinishRun(db, runID, "failed", &exit, output); err != nil {
		t.Fatal(err)
	}
	failures, err := GetFailures(db, runID, 10)
	if err != nil {
		t.Fatal(err)
	}
	_ = failures
	_, err = CollectInputsForScope(db, ".")
	if err != nil {
		t.Fatal(err)
	}
}
