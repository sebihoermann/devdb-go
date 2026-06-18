package inventory

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/domain/architecture"
	"github.com/sebihoermann/devdb-go/internal/domain/feedback"
	"github.com/sebihoermann/devdb-go/internal/domain/planning"
	"github.com/sebihoermann/devdb-go/internal/domain/review"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestContextStrictExitWithHighFinding(t *testing.T) {
	db, _ := testutil.TempDB(t)
	repo := t.TempDir()
	mainGo := filepath.Join(repo, "main.go")
	if err := os.WriteFile(mainGo, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	testutil.InitGitRepo(t, repo)
	runID, _ := review.StartRun(db, []string{"."}, "default", "", "test")
	_, _ = review.AddFinding(db, runID, review.FindingInput{
		FilePath: "main.go", Principle: "kiss", Title: "issue", Recommendation: "fix",
		Severity: "high", Confidence: "high", Effort: "small",
	}, "test")
	_, _ = review.FinishRun(db, runID, "done")
	payload, err := Context(db, ContextOptions{Files: []string{"main.go"}})
	if err != nil {
		t.Fatal(err)
	}
	if !ContextStrictExit(payload) {
		t.Fatalf("expected strict exit: %+v", payload)
	}
	lines := FormatContextHuman(payload)
	if len(lines) == 0 {
		t.Fatal("empty human context")
	}
}

func TestScanInventoryAndLocGitAware(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "a.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "b.py"), []byte("print(1)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	testutil.InitGitRepo(t, repo)
	records, err := ScanInventory(repo, []string{"."}, true)
	if err != nil || len(records) == 0 {
		t.Fatalf("scan inv=%d err=%v", len(records), err)
	}
	loc, err := Loc(repo, []string{"."}, true)
	if err != nil || loc.Files == 0 {
		t.Fatalf("loc=%+v err=%v", loc, err)
	}
}

func TestDiffSinceAndFreshnessWithScan(t *testing.T) {
	db, _ := testutil.TempDB(t)
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	testutil.InitGitRepo(t, repo)
	if _, err := Scan(db, repo, []string{"."}, false, "test"); err != nil {
		t.Fatal(err)
	}
	rows, err := DiffSince(db, repo, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	_ = rows
	fresh, err := FreshnessInfo(db)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.LastScanAt == "" {
		t.Fatalf("freshness=%+v", fresh)
	}
}

func TestContextWithArchNotesAndPlanItem(t *testing.T) {
	db, _ := testutil.TempDB(t)
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "svc.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Scan(db, repo, []string{"."}, false, "test"); err != nil {
		t.Fatal(err)
	}
	_, err := architecture.Add(db, "entry-point", "body text here for arch.", []string{"svc.go"}, "high", "test")
	if err != nil {
		t.Fatal(err)
	}
	planID, _ := planning.CreatePlan(db, planning.CreatePlanInput{Slug: "ctx", Title: "Ctx", ModelID: "test"})
	itemID, _ := planning.AddItem(db, planning.AddItemInput{PlanID: planID, Title: "touch svc", ModelID: "test"})
	_, _ = feedback.Add(db, feedback.AddInput{Role: "model", Note: "related", ModelID: "test"})
	payload, err := Context(db, ContextOptions{Files: []string{"svc.go"}, PlanItemID: itemID})
	if err != nil {
		t.Fatal(err)
	}
	if len(payload.ArchitectureNotes) == 0 {
		t.Fatalf("payload=%+v", payload)
	}
}
