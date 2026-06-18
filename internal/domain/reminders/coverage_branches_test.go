package reminders

import (
	"testing"

	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestReminderDismissSnoozeOverdue(t *testing.T) {
	db, _ := testutil.TempDB(t)
	past := "2020-01-01T00:00:00Z"
	future := "2099-01-01T00:00:00Z"
	overdueID, _ := Add(db, AddInput{Title: "late", DueAt: past, ModelID: "test"})
	openID, _ := Add(db, AddInput{Title: "future", DueAt: future, ModelID: "test"})
	overdue, err := List(db, "open", true, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(overdue) < 1 {
		t.Fatalf("overdue=%+v", overdue)
	}
	_ = overdueID
	if _, err := Snooze(db, overdueID, future); err != nil {
		t.Fatal(err)
	}
	if _, err := Unsnooze(db, openID); err != nil {
		t.Fatal(err)
	}
	if _, err := Dismiss(db, openID); err != nil {
		t.Fatal(err)
	}
	dismissed, err := List(db, "dismissed", false, 10)
	if err != nil || len(dismissed) != 1 {
		t.Fatalf("dismissed=%d err=%v", len(dismissed), err)
	}
}
