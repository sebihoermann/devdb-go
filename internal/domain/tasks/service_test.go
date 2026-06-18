package tasks

import (
	"testing"

	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestTaskCRUD(t *testing.T) {
	db, _ := testutil.TempDB(t)
	id, err := Add(db, "Ship it", "body", "", "", "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SetStatus(db, id, "bogus", "test"); err == nil {
		t.Fatal("expected invalid status")
	}
	if _, err := SetStatus(db, id, "done", "test"); err != nil {
		t.Fatal(err)
	}
	row, err := Show(db, id[:8])
	if err != nil || row.Status != "done" {
		t.Fatalf("show: %+v err=%v", row, err)
	}
	rows, err := List(db, "done", "", 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("list done: %d err=%v", len(rows), err)
	}
	rows, err = List(db, "all", "high", 10)
	if err != nil {
		t.Fatal(err)
	}
	_, err = Add(db, "Low", "", "low", "", "test")
	if err != nil {
		t.Fatal(err)
	}
	rows, err = List(db, "", "", -1)
	if err != nil || len(rows) < 2 {
		t.Fatalf("default limit: %d", len(rows))
	}
}
