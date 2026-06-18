package status_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/domain/feedback"
	"github.com/sebihoermann/devdb-go/internal/domain/planning"
	"github.com/sebihoermann/devdb-go/internal/domain/status"
	"github.com/sebihoermann/devdb-go/internal/storage"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestQualitySignals(t *testing.T) {
	db, _ := testutil.TempDB(t)

	if _, err := feedback.Add(db, feedback.AddInput{
		Role: "codebase", Severity: "high", Note: "urgent", ModelID: "test",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := feedback.Add(db, feedback.AddInput{
		Role: "user", Severity: "low", Note: "minor", ModelID: "test",
	}); err != nil {
		t.Fatal(err)
	}

	q, err := status.Quality(db)
	if err != nil {
		t.Fatal(err)
	}
	if q.OpenHighFeedback < 1 {
		t.Fatalf("open high feedback=%d want >=1", q.OpenHighFeedback)
	}
}

func TestBuildIdle(t *testing.T) {
	db, _ := testutil.TempDB(t)
	repo := t.TempDir()

	snap, err := status.Build(db, repo, string(storage.SchemaGo), 1)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Overall != "idle" {
		t.Fatalf("overall=%q want idle", snap.Overall)
	}
	if snap.SchemaKind != string(storage.SchemaGo) {
		t.Fatalf("schema kind=%q", snap.SchemaKind)
	}
	if snap.InFlight != nil {
		t.Fatal("expected no in-flight item")
	}
}

func TestBuildWithGitRepoAndInFlight(t *testing.T) {
	db, _ := testutil.TempDB(t)
	repo := t.TempDir()
	testutil.InitGitRepo(t, repo)

	planID, err := planning.CreatePlan(db, planning.CreatePlanInput{
		Slug: "ship", Title: "Ship feature", ModelID: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	itemID, err := planning.AddItem(db, planning.AddItemInput{
		PlanID: planID, Title: "Implement", ModelID: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := planning.StartItem(db, itemID, "test"); err != nil {
		t.Fatal(err)
	}

	snap, err := status.Build(db, repo, string(storage.SchemaGo), 5)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Overall != "active" {
		t.Fatalf("overall=%q want active", snap.Overall)
	}
	if snap.InFlight == nil || snap.InFlight.ID != itemID {
		t.Fatalf("in-flight=%+v want %s", snap.InFlight, itemID)
	}
	if snap.InProgress != 1 {
		t.Fatalf("in_progress=%d", snap.InProgress)
	}
	if snap.GitBranch == "" {
		t.Fatal("expected git branch")
	}
}

func TestBuildPlannedWithOpenFeedback(t *testing.T) {
	db, _ := testutil.TempDB(t)
	repo := t.TempDir()

	planID, err := planning.CreatePlan(db, planning.CreatePlanInput{
		Slug: "plan", Title: "Plan", ModelID: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := planning.AddItem(db, planning.AddItemInput{
		PlanID: planID, Title: "Task", ModelID: "test",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := feedback.Add(db, feedback.AddInput{
		Role: "user", Note: "open item", ModelID: "test",
	}); err != nil {
		t.Fatal(err)
	}

	snap, err := status.Build(db, repo, string(storage.SchemaGo), 1)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Overall != "planned" {
		t.Fatalf("overall=%q want planned", snap.Overall)
	}
	if snap.OpenItems < 1 || snap.OpenFeedback < 1 {
		t.Fatalf("open items=%d feedback=%d", snap.OpenItems, snap.OpenFeedback)
	}
}

func TestBuildDirtyWorktree(t *testing.T) {
	db, _ := testutil.TempDB(t)
	repo := t.TempDir()
	testutil.InitGitRepo(t, repo)

	planID, err := planning.CreatePlan(db, planning.CreatePlanInput{
		Slug: "x", Title: "X", ModelID: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	itemID, err := planning.AddItem(db, planning.AddItemInput{
		PlanID: planID, Title: "Work", ModelID: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := planning.StartItem(db, itemID, "test"); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(repo, "dirty.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	snap, err := status.Build(db, repo, string(storage.SchemaGo), 1)
	if err != nil {
		t.Fatal(err)
	}
	if !snap.GitDirty {
		t.Fatal("expected dirty worktree")
	}
	if snap.Overall != "active · dirty worktree" {
		t.Fatalf("overall=%q", snap.Overall)
	}
}