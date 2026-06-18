package architecture

import (
	"testing"

	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestErrorPathsClosedDB(t *testing.T) {
	db := testutil.ClosedDB(t)
	calls := []func() error{
		func() error { _, err := Add(db, "valid-topic", "body text.", []string{"a.go"}, "medium", "t"); return err },
		func() error { _, err := List(db, ListFilter{}); return err },
		func() error { _, _, _, err := Verify(db, "x"); return err },
		func() error { _, err := CountStale(db); return err },
		func() error { _, err := VerifyAll(db); return err },
		func() error { _, err := RenderMarkdown(db); return err },
	}
	for i, fn := range calls {
		if err := fn(); err == nil {
			t.Fatalf("call %d: expected error", i)
		}
	}
}
