package goals

import (
	"testing"

	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestGoalCRUD(t *testing.T) {
	db, _ := testutil.TempDB(t)
	if _, err := Add(db, "bad", "t", "", "test"); err == nil {
		t.Fatal("expected invalid kind")
	}
	id, err := Add(db, "goal", "Ship M1", "details", "test")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := List(db, "active", 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("list: %d err=%v", len(rows), err)
	}
	if _, err := SetStatus(db, id, "bogus", "test"); err == nil {
		t.Fatal("expected invalid status")
	}
	if _, err := SetStatus(db, id, "done", "test"); err != nil {
		t.Fatal(err)
	}
	rows, err = List(db, "all", 0)
	if err != nil || len(rows) != 1 {
		t.Fatalf("all: %d", len(rows))
	}
}
