package reminders

import (
	"testing"

	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestReminderResolveAndListFilters(t *testing.T) {
	db, _ := testutil.TempDB(t)
	id, _ := Add(db, AddInput{Title: "R", ModelID: "test"})
	got, err := resolveID(db, id)
	if err != nil || got != id {
		t.Fatalf("full id=%q err=%v", got, err)
	}
	pref, err := resolveID(db, id[:8])
	if err != nil || pref != id {
		t.Fatalf("prefix=%q err=%v", pref, err)
	}
	rows, err := List(db, "all", false, -1)
	if err != nil || len(rows) != 1 {
		t.Fatalf("all=%d err=%v", len(rows), err)
	}
	_, err = Show(db, "missing")
	if err == nil {
		t.Fatal("expected missing")
	}
}
