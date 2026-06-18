package approval

import (
	"testing"

	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestErrorPathsClosedDB(t *testing.T) {
	db := testutil.ClosedDB(t)
	calls := []func() error{
		func() error { _, err := Request(db, "tasks", "x", "", "t"); return err },
		func() error { _, err := Approve(db, "tasks", "x", "", "t"); return err },
		func() error { _, err := Reject(db, "tasks", "x", "", "t"); return err },
		func() error { _, err := Withdraw(db, "tasks", "x", "", "t"); return err },
		func() error { _, err := ListPending(db); return err },
		func() error { _, err := Log(db, 10); return err },
	}
	for i, fn := range calls {
		if err := fn(); err == nil {
			t.Fatalf("call %d: expected error", i)
		}
	}
}

func TestLogActionInvalidTable(t *testing.T) {
	db, _ := testutil.TempDB(t)
	if _, err := Request(db, "features", "x", "", "t"); err == nil {
		t.Fatal("expected invalid table error")
	}
}

func TestLogNegativeLimitUsesDefault(t *testing.T) {
	db, _ := testutil.TempDB(t)
	rows, err := Log(db, -1)
	if err != nil {
		t.Fatal(err)
	}
	if rows == nil {
		rows = []LogRow{}
	}
	if len(rows) != 0 {
		t.Fatalf("rows=%d", len(rows))
	}
}
