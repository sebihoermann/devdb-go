package reminders

import (
	"testing"
	"time"

	"github.com/sebihoermann/devdb-go/internal/storage"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestReminderCRUD(t *testing.T) {
	db, _ := testutil.TempDB(t)
	past := time.Now().UTC().Add(-2 * time.Hour).Format("2006-01-02 15:04:05")
	future := time.Now().UTC().Add(24 * time.Hour).Format("2006-01-02 15:04:05")
	id, err := Add(db, AddInput{Title: "Due soon", DueAt: past, FilePath: "pkg/a.go", ModelID: "test"})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := List(db, "open", true, 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("overdue: %d err=%v", len(rows), err)
	}
	if !IsOverdue(rows[0]) {
		t.Fatal("expected overdue")
	}
	fileRows, err := ListForFile(db, "pkg/a.go")
	if err != nil || len(fileRows) != 1 {
		t.Fatalf("for file: %d", len(fileRows))
	}
	if _, err := Snooze(db, id, ""); err == nil {
		t.Fatal("expected until required")
	}
	if _, err := Snooze(db, id, future); err != nil {
		t.Fatal(err)
	}
	snoozed, err := Show(db, id[:8])
	if err != nil || !IsOverdue(snoozed) == false {
		// snoozed should not be overdue if snooze is in future
		if IsOverdue(snoozed) {
			t.Fatal("snoozed reminder should not be overdue")
		}
	}
	if _, err := Unsnooze(db, id); err != nil {
		t.Fatal(err)
	}
	if _, err := Dismiss(db, id); err != nil {
		t.Fatal(err)
	}
	rows, err = List(db, "dismissed", false, 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("dismissed: %d", len(rows))
	}
}

func TestListForPlanItem(t *testing.T) {
	db, _ := testutil.TempDB(t)
	planItemID, _ := storage.NewID()
	id, err := Add(db, AddInput{Title: "Plan reminder", PlanItemID: planItemID, ModelID: "test"})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := ListForPlanItem(db, planItemID)
	if err != nil || len(rows) != 1 || rows[0].ID != id {
		t.Fatalf("plan item rows: %+v err=%v", rows, err)
	}
}
