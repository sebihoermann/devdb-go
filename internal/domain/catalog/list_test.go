package catalog

import (
	"testing"

	"github.com/sebihoermann/devdb-go/internal/domain/feedback"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestListAndShow(t *testing.T) {
	db, _ := testutil.TempDB(t)
	if _, err := ListRows(db, "nope", 10); err == nil {
		t.Fatal("expected unknown table")
	}
	id, err := feedback.Add(db, feedback.AddInput{Role: "user", Note: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := ListRows(db, "feedback", 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("list: %d err=%v", len(rows), err)
	}
	row, err := ShowRow(db, "feedback", id[:8])
	if err != nil {
		t.Fatal(err)
	}
	if row["id"] != id {
		t.Fatalf("id=%v", row["id"])
	}
	if _, err := ShowRow(db, "feedback", "zzz"); err == nil {
		t.Fatal("expected no match")
	}
	rows, err = ListRows(db, "feedback", 0)
	if err != nil || len(rows) != 1 {
		t.Fatalf("unlimited: %d", len(rows))
	}
}
