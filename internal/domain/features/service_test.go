package features

import (
	"testing"

	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestFeatureAddList(t *testing.T) {
	db, _ := testutil.TempDB(t)
	id, err := Add(db, "Feature X", "desc", "abc", "main", "test")
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Fatal("empty id")
	}
	rows, err := List(db, 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("list: %d err=%v", len(rows), err)
	}
	if rows[0].Title != "Feature X" {
		t.Fatalf("title=%q", rows[0].Title)
	}
	rows, err = List(db, -1)
	if err != nil || len(rows) != 1 {
		t.Fatalf("default limit: %d", len(rows))
	}
	_, err = Add(db, "Bare", "", "", "", "test")
	if err != nil {
		t.Fatal(err)
	}
	all, err := List(db, 0)
	if err != nil || len(all) != 2 {
		t.Fatalf("unlimited: %d", len(all))
	}
}
