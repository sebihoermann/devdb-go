package tasks

import (
	"testing"

	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestErrorPathsClosedDB(t *testing.T) {
	db := testutil.ClosedDB(t)
	if _, err := Add(db, "t", "", "", "", "m"); err == nil {
		t.Fatal("add")
	}
	if _, err := List(db, "", "", 0); err == nil {
		t.Fatal("list")
	}
	if _, err := Show(db, "x"); err == nil {
		t.Fatal("show")
	}
	if _, err := SetStatus(db, "x", "done", "m"); err == nil {
		t.Fatal("set")
	}
}
