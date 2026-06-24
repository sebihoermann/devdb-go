package planning_test

import (
	"database/sql"
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

func TestCloseLastItemCascadesToPlan(t *testing.T) {
	db, _ := testutil.TempDB(t)
	planID, _ := planning.CreatePlan(db, planning.CreatePlanInput{Slug: "cascade", Title: "Cascade", ModelID: "test"})
	msID, _ := planning.AddMilestone(db, planID, "M1", "", "test", 1)
	itemID, _ := planning.AddStructuredItem(db, planning.StructuredItemInput{
		PlanID: planID, MilestoneID: msID, Title: "Only item", ModelID: "test",
	})
	accID, _ := planning.AddAcceptance(db, itemID, "ship", "test", 1)
	if _, err := planning.MeetAcceptance(db, accID, "ok", "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := planning.CloseItem(db, itemID, "done", "test"); err != nil {
		t.Fatal(err)
	}
	if status := planStatus(t, db, planID); status != "done" {
		t.Fatalf("plan status=%q after CloseItem; auto-cascade expected done", status)
	}
	if status := milestoneStatus(t, db, msID); status != "done" {
		t.Fatalf("milestone status=%q after CloseItem; auto-cascade expected done", status)
	}
}

func TestCloseLastItemCascadesAcrossMilestones(t *testing.T) {
	db, _ := testutil.TempDB(t)
	planID, _ := planning.CreatePlan(db, planning.CreatePlanInput{Slug: "cascade-multi", Title: "Cascade Multi", ModelID: "test"})
	ms1, _ := planning.AddMilestone(db, planID, "M1", "", "test", 1)
	ms2, _ := planning.AddMilestone(db, planID, "M2", "", "test", 2)

	item1, _ := planning.AddStructuredItem(db, planning.StructuredItemInput{
		PlanID: planID, MilestoneID: ms1, Title: "A", ModelID: "test",
	})
	acc1, _ := planning.AddAcceptance(db, item1, "a", "test", 1)
	if _, err := planning.MeetAcceptance(db, acc1, "x", "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := planning.CloseItem(db, item1, "ship", "test"); err != nil {
		t.Fatal(err)
	}
	if status := planStatus(t, db, planID); status != "active" {
		t.Fatalf("plan should still be active with M2 open; got %q", status)
	}

	item2, _ := planning.AddStructuredItem(db, planning.StructuredItemInput{
		PlanID: planID, MilestoneID: ms2, Title: "B", ModelID: "test",
	})
	acc2, _ := planning.AddAcceptance(db, item2, "b", "test", 1)
	if _, err := planning.MeetAcceptance(db, acc2, "x", "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := planning.CloseItem(db, item2, "ship", "test"); err != nil {
		t.Fatal(err)
	}
	if status := milestoneStatus(t, db, ms2); status != "done" {
		t.Fatalf("M2 should auto-cascade to done; got %q", status)
	}
	if status := planStatus(t, db, planID); status != "done" {
		t.Fatalf("plan should auto-cascade to done after last item; got %q", status)
	}
}

func planStatus(t *testing.T, db *sql.DB, id string) string {
	t.Helper()
	var status string
	if err := db.QueryRow(`SELECT status FROM plans WHERE id=?`, id).Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

func milestoneStatus(t *testing.T, db *sql.DB, id string) string {
	t.Helper()
	var status string
	if err := db.QueryRow(`SELECT status FROM milestones WHERE id=?`, id).Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
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
	if items[0].MemoryRef != "MEMORY.md#task" {
		t.Fatalf("list memory_ref=%q", items[0].MemoryRef)
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
