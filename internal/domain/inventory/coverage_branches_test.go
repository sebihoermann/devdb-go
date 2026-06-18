package inventory

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/domain/planning"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestContextAndDiffBranches(t *testing.T) {
	repo := t.TempDir()
	mainGo := filepath.Join(repo, "main.go")
	if err := os.WriteFile(mainGo, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	testutil.InitGitRepo(t, repo)
	db, _ := testutil.TempDB(t)
	if _, err := Scan(db, repo, nil, false, "test"); err != nil {
		t.Fatal(err)
	}
	planID, _ := planning.CreatePlan(db, planning.CreatePlanInput{Slug: "ctx", Title: "Ctx", ModelID: "test"})
	msID, _ := planning.AddMilestone(db, planID, "M1", "", "test", 1)
	itemID, _ := planning.AddStructuredItem(db, planning.StructuredItemInput{
		PlanID: planID, MilestoneID: msID, Title: "Work", ModelID: "test",
	})
	payload, err := Context(db, ContextOptions{Files: []string{"main.go"}, PlanItemID: itemID})
	if err != nil || len(payload.Files) == 0 {
		t.Fatalf("context=%+v err=%v", payload, err)
	}
	diff, err := DiffSince(db, repo, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	_ = diff
}

func TestLocGitAware(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "tracked.go"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	testutil.InitGitRepo(t, repo)
	sum, err := Loc(repo, nil, true)
	if err != nil || sum.Files < 1 {
		t.Fatalf("loc=%+v err=%v", sum, err)
	}
}

func TestContextEmptyFilesUsesActiveNotes(t *testing.T) {
	db, _ := testutil.TempDB(t)
	repo := t.TempDir()
	p := filepath.Join(repo, "n.go")
	if err := os.WriteFile(p, []byte("package n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _ = Scan(db, repo, nil, false, "test")
	payload, err := Context(db, ContextOptions{})
	if err != nil {
		t.Fatal(err)
	}
	_ = payload
}
