package reminders

import (
	"testing"

	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestErrorPathsClosedDB(t *testing.T) {
	db := testutil.ClosedDB(t)
	if _, err := Add(db, AddInput{Title: "r", ModelID: "m"}); err == nil {
		t.Fatal("add")
	}
	if _, err := List(db, "", false, 0); err == nil {
		t.Fatal("list")
	}
	if _, err := Show(db, "x"); err == nil {
		t.Fatal("show")
	}
	if _, err := Dismiss(db, "x"); err == nil {
		t.Fatal("dismiss")
	}
	if _, err := Snooze(db, "x", "2099-01-01T00:00:00Z"); err == nil {
		t.Fatal("snooze")
	}
	if _, err := Unsnooze(db, "x"); err == nil {
		t.Fatal("unsnooze")
	}
}
