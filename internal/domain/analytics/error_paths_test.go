package analytics

import (
	"testing"

	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestErrorPathsClosedDB(t *testing.T) {
	db := testutil.ClosedDB(t)
	if _, err := ListMissedCalls(db, "", 10); err == nil {
		t.Fatal("expected list error")
	}
	if _, err := MissedSummary(db, "", 7); err == nil {
		t.Fatal("expected summary error")
	}
	if _, err := Hygiene(db); err == nil {
		t.Fatal("expected hygiene error")
	}
}
