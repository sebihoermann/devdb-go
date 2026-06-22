package planning_test

import (
	"path/filepath"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/domain/planning"
	"github.com/sebihoermann/devdb-go/internal/migrate"
	"github.com/sebihoermann/devdb-go/internal/storage"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestCloseItemRequiresMetAcceptance(t *testing.T) {
	dir := t.TempDir()
	db, err := storage.Open(filepath.Join(dir, "development.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := migrate.RunAll(db); err != nil {
		t.Fatal(err)
	}

	itemID, err := planning.AddLegacyItem(db, "M1", "1", "test", "", "", "test")
	if err != nil {
		t.Fatal(err)
	}
	_, err = planning.AddAcceptance(db, itemID, "must pass", "test", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := planning.CloseItem(db, itemID, "nope", "test"); err == nil {
		t.Fatal("expected close to fail with open acceptance")
	}
	accRows, _ := db.Query(`SELECT id FROM plan_item_acceptance WHERE plan_item_id=?`, itemID)
	var accID string
	if accRows.Next() {
		_ = accRows.Scan(&accID)
	}
	accRows.Close()
	if _, err := planning.MeetAcceptance(db, accID, "test evidence", "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := planning.CloseItem(db, itemID, "done", "test"); err != nil {
		t.Fatal(err)
	}
}

func TestListItemsShowItemAndInFlight(t *testing.T) {
	db, _ := testutil.TempDB(t)
	planID, _ := planning.CreatePlan(db, planning.CreatePlanInput{Slug: "list", Title: "List", ModelID: "test"})
	msID, _ := planning.AddMilestone(db, planID, "M1", "", "test", 1)
	itemID, _ := planning.AddItem(db, planning.AddItemInput{PlanID: planID, MilestoneID: msID, Title: "Task", MemoryRef: "MEMORY.md#task", ModelID: "test"})
	_, _ = planning.StartItem(db, itemID, "test")

	items, err := planning.ListItems(db, planning.ItemFilter{PlanID: planID})
	if err != nil || len(items) != 1 {
		t.Fatalf("items=%d err=%v", len(items), err)
	}
	show, acc, err := planning.ShowItem(db, itemID[:8])
	if err != nil || show.ID != itemID {
		t.Fatalf("show=%+v err=%v", show, err)
	}
	if show.MemoryRef != "MEMORY.md#task" {
		t.Fatalf("memory_ref=%q", show.MemoryRef)
	}
	_ = acc
	inFlight, err := planning.InFlight(db)
	if err != nil || inFlight == nil || inFlight.ID != itemID {
		t.Fatalf("in-flight=%+v err=%v", inFlight, err)
	}
	planned, err := planning.ListItems(db, planning.ItemFilter{Status: "planned"})
	if err != nil {
		t.Fatal(err)
	}
	if len(planned) != 0 {
		t.Fatalf("planned=%d", len(planned))
	}
}
