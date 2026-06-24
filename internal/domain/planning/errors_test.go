package planning_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/domain/planning"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

// TestSentinelErrorsMatchWrapping ensures callers using errors.Is still match
// when the domain function wraps the sentinel with extra context.
// Regression for review finding 86b1ba54 (fragile string matching).
func TestSentinelErrorsMatchWrapping(t *testing.T) {
	t.Run("ErrSlugExists wraps with slug detail", func(t *testing.T) {
		db, _ := testutil.TempDB(t)
		if _, err := planning.CreatePlan(db, planning.CreatePlanInput{Slug: "dupe", Title: "D", ModelID: "test"}); err != nil {
			t.Fatal(err)
		}
		_, err := planning.ScaffoldPlan(db, planning.ScaffoldPlanInput{Title: "Title", Slug: "dupe", Mode: "implement", ModelID: "test"})
		if err == nil {
			t.Fatal("expected slug collision")
		}
		if !errors.Is(err, planning.ErrSlugExists) {
			t.Fatalf("err=%v does not wrap ErrSlugExists", err)
		}
		if !strings.Contains(err.Error(), "dupe") {
			t.Fatalf("err=%v missing slug detail", err)
		}
	})

	t.Run("ErrInvalidMode is the bare sentinel", func(t *testing.T) {
		db, _ := testutil.TempDB(t)
		_, err := planning.ScaffoldPlan(db, planning.ScaffoldPlanInput{Title: "Title", Slug: "x", Mode: "bogus", ModelID: "test"})
		if err == nil {
			t.Fatal("expected invalid mode")
		}
		if !errors.Is(err, planning.ErrInvalidMode) {
			t.Fatalf("err=%v does not match ErrInvalidMode", err)
		}
	})

	t.Run("ErrSpecFileNotFound wraps with path detail", func(t *testing.T) {
		db, _ := testutil.TempDB(t)
		_, err := planning.BackfillAcceptanceFromSpec(db, "M1", "/no/such/path.md", "test")
		if err == nil {
			t.Fatal("expected missing spec")
		}
		if !errors.Is(err, planning.ErrSpecFileNotFound) {
			t.Fatalf("err=%v does not wrap ErrSpecFileNotFound", err)
		}
		if !strings.Contains(err.Error(), "/no/such/path.md") {
			t.Fatalf("err=%v missing path detail", err)
		}
	})

	t.Run("ErrPlanNotFound wraps with ref detail", func(t *testing.T) {
		db, _ := testutil.TempDB(t)
		_, err := planning.ResolvePlanID(db, "missing")
		if err == nil {
			t.Fatal("expected not found")
		}
		if !errors.Is(err, planning.ErrPlanNotFound) {
			t.Fatalf("err=%v does not wrap ErrPlanNotFound", err)
		}
	})

	t.Run("ErrMilestoneNotFound wraps with ref detail", func(t *testing.T) {
		db, _ := testutil.TempDB(t)
		planID, _ := planning.CreatePlan(db, planning.CreatePlanInput{Slug: "no-ms", Title: "NM", ModelID: "test"})
		_, _, err := planning.ResolveMilestoneID(db, planID, "M9")
		if err == nil {
			t.Fatal("expected milestone not found")
		}
		if !errors.Is(err, planning.ErrMilestoneNotFound) {
			t.Fatalf("err=%v does not wrap ErrMilestoneNotFound", err)
		}
	})

	t.Run("ErrNoteRequired surfaces from PauseItem", func(t *testing.T) {
		db, _ := testutil.TempDB(t)
		planID, _ := planning.CreatePlan(db, planning.CreatePlanInput{Slug: "p", Title: "P", ModelID: "test"})
		itemID, _ := planning.AddItem(db, planning.AddItemInput{PlanID: planID, Title: "task", ModelID: "test"})
		if _, err := planning.StartItem(db, itemID, "test"); err != nil {
			t.Fatal(err)
		}
		_, err := planning.PauseItem(db, itemID, "   ", "test")
		if err == nil {
			t.Fatal("expected note required")
		}
		if !errors.Is(err, planning.ErrNoteRequired) {
			t.Fatalf("err=%v does not match ErrNoteRequired", err)
		}
	})

	// Regression for feedback ad7073e8: PauseItem used to silently flip a
	// 'planned' item to 'in_progress', contaminating the next session's
	// resume. Pause must now require an explicit StartItem first.
	t.Run("ErrItemNotInProgress when pausing a planned item", func(t *testing.T) {
		db, _ := testutil.TempDB(t)
		planID, _ := planning.CreatePlan(db, planning.CreatePlanInput{Slug: "p", Title: "P", ModelID: "test"})
		itemID, _ := planning.AddItem(db, planning.AddItemInput{PlanID: planID, Title: "task", ModelID: "test"})
		_, err := planning.PauseItem(db, itemID, "save context", "test")
		if err == nil {
			t.Fatal("expected pause rejection on planned item")
		}
		if !errors.Is(err, planning.ErrItemNotInProgress) {
			t.Fatalf("err=%v does not wrap ErrItemNotInProgress", err)
		}
		if !strings.Contains(err.Error(), "planned") {
			t.Fatalf("err=%v should mention the current status", err)
		}
		var status string
		if err := db.QueryRow(`SELECT status FROM plan_items WHERE id=?`, itemID).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status != "planned" {
			t.Fatalf("plan item status flipped to %q after rejected pause", status)
		}
	})

	t.Run("PauseItem succeeds after StartItem", func(t *testing.T) {
		db, _ := testutil.TempDB(t)
		planID, _ := planning.CreatePlan(db, planning.CreatePlanInput{Slug: "p", Title: "P", ModelID: "test"})
		itemID, _ := planning.AddItem(db, planning.AddItemInput{PlanID: planID, Title: "task", ModelID: "test"})
		if _, err := planning.StartItem(db, itemID, "test"); err != nil {
			t.Fatal(err)
		}
		if _, err := planning.PauseItem(db, itemID, "save context", "test"); err != nil {
			t.Fatalf("pause after start should succeed, got %v", err)
		}
	})
}