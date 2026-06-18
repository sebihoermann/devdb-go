package review

import (
	"testing"

	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestErrorPathsClosedDBExtended(t *testing.T) {
	db := testutil.ClosedDB(t)
	if _, err := StartRun(db, []string{"."}, "default", "", "t"); err == nil {
		t.Fatal("start run")
	}
	if p := PrinciplesForTier("grass-cutter"); len(p) == 0 {
		t.Fatal("expected principles")
	}
	if p := PrinciplesForTier("unknown-tier"); len(p) == 0 {
		t.Fatal("expected default principles")
	}
}
