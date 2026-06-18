package archive

import (
	"testing"

	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestErrorPathsClosedDBExtended(t *testing.T) {
	db := testutil.ClosedDB(t)
	calls := []func() error{
		func() error { _, err := countRows(db, "feedback", "", nil, "", 0); return err },
		func() error { _, err := countLocSnapshots(db, 1); return err },
		func() error { _, err := readTableRows(db, `SELECT id FROM feedback`); return err },
		func() error {
			return archiveRow(db, "feedback", "x", map[string]any{"id": "x"}, "now", "test")
		},
	}
	for i, fn := range calls {
		if err := fn(); err == nil {
			t.Fatalf("call %d: expected error", i)
		}
	}
}
