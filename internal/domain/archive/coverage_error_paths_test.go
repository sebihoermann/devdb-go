package archive

import (
	"testing"

	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestErrorPathsClosedDBGCAndRestore(t *testing.T) {
	db := testutil.ClosedDB(t)
	if _, err := GC(db, GCOptions{DryRun: true}); err == nil {
		t.Fatal("gc")
	}
	if _, err := Restore(db, RestoreOptions{ID: "x"}); err == nil {
		t.Fatal("restore")
	}
	if _, err := Run(db, RunOptions{Yes: true, Vacuum: true}); err == nil {
		t.Fatal("run")
	}
}
