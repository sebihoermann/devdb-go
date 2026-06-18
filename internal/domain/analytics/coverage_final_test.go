package analytics

import (
	"testing"

	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestRecordMissedCallAndSummaryBranches(t *testing.T) {
	db, _ := testutil.TempDB(t)
	RecordMissedCall(db, []string{"devdb", "status"}, "usage", "bad args", "devdb help", 2, "/tmp", "/repo", "test")
	RecordMissedCall(nil, nil, "", "", "", 0, "", "", "")
	rows, err := ListMissedCalls(db, "", 10)
	if err != nil || len(rows) == 0 {
		t.Fatalf("missed=%d err=%v", len(rows), err)
	}
	sum, err := MissedSummary(db, "", 7)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Total < 1 {
		t.Fatalf("summary=%+v", sum)
	}
}

func TestHygieneBranches(t *testing.T) {
	db, _ := testutil.TempDB(t)
	report, err := Hygiene(db)
	if err != nil {
		t.Fatal(err)
	}
	if report.MissedCalls7d < 0 {
		t.Fatalf("hygiene=%+v", report)
	}
}
