package planning

import (
	"testing"

	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestSlugifyAndNullStr(t *testing.T) {
	if slugify("Hello World!") != "hello-world" {
		t.Fatalf("slug=%q", slugify("Hello World!"))
	}
	if nullStr("") != nil {
		t.Fatal("empty should be nil")
	}
	if v, ok := nullStr("x").(string); !ok || v != "x" {
		t.Fatalf("nullStr=%v", nullStr("x"))
	}
}

func TestNextItemNumberAndResolveItemID(t *testing.T) {
	db, _ := testutil.TempDB(t)
	planID, _ := CreatePlan(db, CreatePlanInput{Slug: "n", Title: "N", ModelID: "test"})
	msID, _ := AddMilestone(db, planID, "M1", "", "test", 1)
	n1, err := nextItemNumber(db, planID, msID)
	if err != nil || n1 != 1 {
		t.Fatalf("n1=%d err=%v", n1, err)
	}
	itemID, _ := AddStructuredItem(db, StructuredItemInput{PlanID: planID, MilestoneID: msID, Title: "A", ModelID: "test"})
	resolved, err := resolveItemID(db, itemID[:8])
	if err != nil || resolved != itemID {
		t.Fatalf("resolved=%s err=%v", resolved, err)
	}
}
