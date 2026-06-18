package feedback

import (
	"testing"

	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestErrorPathsClosedDB(t *testing.T) {
	db := testutil.ClosedDB(t)
	calls := []func() error{
		func() error { _, err := Add(db, AddInput{Role: "user", Note: "n"}); return err },
		func() error { _, err := List(db, "", 10); return err },
		func() error { _, err := Show(db, "x"); return err },
		func() error { _, err := Close(db, "x", "", "t"); return err },
		func() error { _, err := Annotate(db, "x", "note", "t"); return err },
		func() error { _, err := ImportMarkdown(db, "/nope.md", "t"); return err },
	}
	for i, fn := range calls {
		if err := fn(); err == nil {
			t.Fatalf("call %d: expected error", i)
		}
	}
}
