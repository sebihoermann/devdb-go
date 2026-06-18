package review

import (
	"testing"

	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestErrorPathsClosedDB(t *testing.T) {
	db := testutil.ClosedDB(t)
	calls := []func() error{
		func() error { _, err := StartRun(db, []string{"."}, "default", "", "t"); return err },
		func() error { _, err := AddFinding(db, "x", FindingInput{Principle: "other", Title: "n"}, "t"); return err },
		func() error { _, err := FinishRun(db, "x", "done"); return err },
		func() error { _, err := ListFindings(db, ListFilter{}); return err },
		func() error { _, err := ResolveFinding(db, "x", "", "resolved", ""); return err },
		func() error { _, err := GetRun(db, "x"); return err },
		func() error { _, err := RenderReport(db, "x"); return err },
	}
	for i, fn := range calls {
		if err := fn(); err == nil {
			t.Fatalf("call %d: expected error", i)
		}
	}
}
