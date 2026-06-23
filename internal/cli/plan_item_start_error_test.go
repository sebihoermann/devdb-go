package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/storage"
)

func TestPlanItemStartPropagatesShowItemError(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "development.db")

	runCLIOut(t, "--db", dbPath, "init")
	planID := strings.TrimSpace(runCLIOut(t, "--db", dbPath, "plan", "create", "Plan", "--slug", "plan"))
	milestoneID := strings.TrimSpace(runCLIOut(t, "--db", dbPath, "plan", "milestone", "add", "Milestone", "--plan", planID))
	itemID := strings.TrimSpace(runCLIOut(t, "--db", dbPath, "plan", "item", "add", "Item", "--plan", planID, "--milestone", milestoneID))

	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`DROP TABLE plan_item_acceptance`); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, "--db", dbPath, "plan", "item", "start", itemID)
	if code == 0 {
		t.Fatalf("expected non-zero exit stderr=%q", stderr)
	}
	if !strings.Contains(stderr, "plan_item_acceptance") {
		t.Fatalf("stderr=%q", stderr)
	}
}
