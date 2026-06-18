package verification

import (
	"testing"

	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestGetFailuresAndReplaceFailures(t *testing.T) {
	db, _ := testutil.TempDB(t)
	exit := 1
	runID, _ := RecordRun(db, "pytest", ".", "", "failed", &exit, "", "", "test")
	output := "FAILED pkg/test_a.py::test_one - AssertionError\nFAILED pkg/test_b.py::test_two - ValueError"
	if err := FinishRun(db, runID, "failed", &exit, output); err != nil {
		t.Fatal(err)
	}
	failures, err := GetFailures(db, runID, 5)
	if err != nil || len(failures) < 1 {
		t.Fatalf("failures=%d err=%v", len(failures), err)
	}
	sum, err := Show(db, runID)
	if err != nil || sum == nil || len(sum.Failures) == 0 {
		t.Fatalf("show=%+v err=%v", sum, err)
	}
}

func TestEvaluateReuseNoPriorRun(t *testing.T) {
	db, _ := testutil.TempDB(t)
	q := Query(db, "make", ".", nil, false)
	if q.Reason == "" {
		t.Fatalf("query=%+v", q)
	}
}
