package inventory

import (
	"testing"

	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestErrorPathsClosedDB(t *testing.T) {
	db := testutil.ClosedDB(t)
	repo := t.TempDir()
	calls := []func() error{
		func() error { _, err := Scan(db, repo, []string{"."}, false, "t"); return err },
		func() error { _, err := Context(db, ContextOptions{}); return err },
		func() error { _, err := DiffSince(db, repo, "HEAD"); return err },
		func() error { _, err := FreshnessInfo(db); return err },
	}
	for i, fn := range calls {
		if err := fn(); err == nil {
			t.Fatalf("call %d: expected error", i)
		}
	}
	_, _ = Loc(repo, []string{"."}, false)
}
