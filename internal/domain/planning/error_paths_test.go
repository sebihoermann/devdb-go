package planning

import (
	"testing"

	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestErrorPathsClosedDB(t *testing.T) {
	db := testutil.ClosedDB(t)
	calls := []func() error{
		func() error { _, err := CreatePlan(db, CreatePlanInput{Slug: "x", Title: "x", ModelID: "t"}); return err },
		func() error { _, err := AddItem(db, AddItemInput{PlanID: "x", Title: "x", ModelID: "t"}); return err },
		func() error { _, err := AddAcceptance(db, "x", "c", "t", 1); return err },
		func() error { return SetItemStatus(db, "x", "open", "", "t") },
		func() error { _, err := StartItem(db, "x", "t"); return err },
		func() error { _, err := PauseItem(db, "x", "", "t"); return err },
		func() error { _, err := InFlight(db); return err },
		func() error { _, _, err := ShowItem(db, "x"); return err },
		func() error { _, err := MeetAcceptance(db, "x", "", "t"); return err },
		func() error { _, err := CloseItem(db, "x", "", "t"); return err },
		func() error { _, err := ListPlans(db); return err },
		func() error { _, err := AddMilestone(db, "x", "m", "", "t", 1); return err },
		func() error { _, err := ListMilestones(db, "x"); return err },
		func() error { _, _, err := ShowPlan(db, "x"); return err },
		func() error { _, err := PlanTree(db, "x"); return err },
		func() error { _, err := ListItems(db, ItemFilter{}); return err },
		func() error { _, err := AddLegacyItem(db, "M", "1", "t", "", "t"); return err },
		func() error { _, err := AddPlanFile(db, "x", "a.go", "modify", "t"); return err },
		func() error { _, err := ListPlanFiles(db, "x"); return err },
		func() error { _, err := SetItemStatusExplicit(db, "x", "open", "", "t"); return err },
		func() error { _, err := FindPlanTreeDrift(db, ""); return err },
		func() error { _, err := ReconcilePlans(db, "", false); return err },
		func() error { _, err := ScaffoldPlan(db, ScaffoldPlanInput{Slug: "s", Title: "t", ModelID: "t"}); return err },
		func() error { _, err := PromotePlan(db, "s", "t"); return err },
		func() error { _, err := BackfillAcceptanceFromSpec(db, "M1", "/nope", "t"); return err },
	}
	for i, fn := range calls {
		if err := fn(); err == nil {
			t.Fatalf("call %d: expected error", i)
		}
	}
}
