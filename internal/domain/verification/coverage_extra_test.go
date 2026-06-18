package verification

import (
	"testing"

	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestErrorPathsClosedDBExtended(t *testing.T) {
	db := testutil.ClosedDB(t)
	calls := []func() error{
		func() error { return AddInputs(db, "x", [][3]string{{"a.go", "scope", "hash"}}, "t") },
		func() error { return FinishRun(db, "x", "passed", nil, "") },
		func() error { _, err := Dismiss(db, "x", "reason"); return err },
		func() error { _, err := GetInputs(db, "x"); return err },
		func() error { _, err := GetFailures(db, "x", 10); return err },
		func() error { _, err := CollectInputsForScope(db, "."); return err },
	}
	for i, fn := range calls {
		if err := fn(); err == nil {
			t.Fatalf("call %d: expected error", i)
		}
	}
}
