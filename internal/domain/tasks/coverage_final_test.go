package tasks

import (
	"testing"

	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestTaskResolveAndDone(t *testing.T) {
	db, _ := testutil.TempDB(t)
	id, _ := Add(db, "Task", "body", "high", "", "test")
	got, err := resolveID(db, id[:8])
	if err != nil || got != id {
		t.Fatalf("resolve=%q err=%v", got, err)
	}
	if _, err := SetStatus(db, id, "bogus", "test"); err == nil {
		t.Fatal("bad status")
	}
	if _, err := SetStatus(db, id, "done", "test"); err != nil {
		t.Fatal(err)
	}
	rows, _ := List(db, "done", "", 0)
	if len(rows) == 0 {
		t.Fatal("expected done task")
	}
}
