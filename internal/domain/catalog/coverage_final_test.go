package catalog

import (
	"testing"

	"github.com/sebihoermann/devdb-go/internal/domain/feedback"
	"github.com/sebihoermann/devdb-go/internal/domain/goals"
	"github.com/sebihoermann/devdb-go/internal/domain/tasks"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestListAndShowAllTables(t *testing.T) {
	db, _ := testutil.TempDB(t)
	goalID, _ := goals.Add(db, "goal", "title", "body", "test")
	fbID, _ := feedback.Add(db, feedback.AddInput{Role: "model", Note: "n", ModelID: "test"})
	taskID, _ := tasks.Add(db, "task", "", "med", "", "test")
	_ = goalID
	_ = fbID
	_ = taskID
	for _, table := range []string{"goals", "feedback", "features", "plans", "plan_items", "tasks", "reminders"} {
		rows, err := ListRows(db, table, 5)
		if err != nil {
			t.Fatalf("list %s: %v", table, err)
		}
		_ = rows
	}
	if _, err := ListRows(db, "nope", 5); err == nil {
		t.Fatal("unknown table")
	}
	row, err := ShowRow(db, "goals", goalID[:8])
	if err != nil || row == nil {
		t.Fatalf("show=%v err=%v", row, err)
	}
	if _, err := ShowRow(db, "goals", "missing"); err == nil {
		t.Fatal("missing row")
	}
}
