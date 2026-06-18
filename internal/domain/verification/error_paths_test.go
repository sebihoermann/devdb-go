package verification

import (
	"testing"

	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestErrorPathsClosedDB(t *testing.T) {
	db := testutil.ClosedDB(t)
	calls := []func() error{
		func() error { _, err := RecordRun(db, "go test", ".", "", "passed", nil, "", "", "t"); return err },
		func() error { return AddInputs(db, "x", [][3]string{{"a.go", "source", "h"}}, "t") },
		func() error { return FinishRun(db, "x", "passed", nil, "") },
		func() error { _, err := Dismiss(db, "x", "reason"); return err },
		func() error { _, err := GetRun(db, "x"); return err },
		func() error { _, err := Show(db, "x"); return err },
		func() error { _, err := listFileChangeEventsSince(db, "2000-01-01"); return err },
	}
	for i, fn := range calls {
		if err := fn(); err == nil {
			t.Fatalf("call %d: expected error", i)
		}
	}
	_, _ = EvaluateFreshness(db, "x")
	_ = Query(db, "go test", ".", nil, false)
}
