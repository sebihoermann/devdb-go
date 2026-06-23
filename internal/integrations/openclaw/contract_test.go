package openclaw

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/app"
	"github.com/sebihoermann/devdb-go/internal/domain/architecture"
	"github.com/sebihoermann/devdb-go/internal/domain/feedback"
	"github.com/sebihoermann/devdb-go/internal/domain/planning"
)

func runAdapterJSON(t *testing.T, workspace, repo string, args ...string) []byte {
	t.Helper()
	var output bytes.Buffer
	cmd := NewCommand()
	cmd.SetOut(&output)
	base := []string{"--workspace", workspace, "--repo", repo, "--json"}
	cmd.SetArgs(append(base, args...))
	if err := cmd.Execute(); err != nil {
		t.Fatalf("args=%v error=%v", args, err)
	}
	return output.Bytes()
}

func TestAdapterContractInNonGitPathsWithSpaces(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace with spaces")
	repo := filepath.Join(root, "target repo with spaces")
	if err := os.MkdirAll(filepath.Join(workspace, "memory"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "MEMORY.md"), []byte("# Memory\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "memory", "2026-06-22.md"), []byte("# Day\nfriction: contract hit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(repo, ".git")); !os.IsNotExist(err) {
		t.Fatalf("target unexpectedly has .git: %v", err)
	}

	ctx, err := app.Open(repo, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := ctx.InitDB(); err != nil {
		t.Fatal(err)
	}
	planID, _ := planning.CreatePlan(ctx.DB, planning.CreatePlanInput{Slug: "contract", Title: "Contract", ModelID: "test"})
	if _, err := planning.AddItem(ctx.DB, planning.AddItemInput{PlanID: planID, Title: "Linked", MemoryRef: "openclaw:MEMORY.md#memory", ModelID: "test"}); err != nil {
		t.Fatal(err)
	}
	_ = ctx.Close()

	var discovery Discovery
	if err := json.Unmarshal(runAdapterJSON(t, workspace, repo, "list"), &discovery); err != nil || len(discovery.Files) == 0 {
		t.Fatalf("discovery=%+v err=%v", discovery, err)
	}
	var firstSync, secondSync SyncResult
	if err := json.Unmarshal(runAdapterJSON(t, workspace, repo, "sync"), &firstSync); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(runAdapterJSON(t, workspace, repo, "sync"), &secondSync); err != nil {
		t.Fatal(err)
	}
	if firstSync.NotesCreated != 2 || secondSync.NotesCreated != 0 || secondSync.NotesUpdated != 2 {
		t.Fatalf("sync first=%+v second=%+v", firstSync, secondSync)
	}
	var firstFriction, secondFriction FrictionResult
	if err := json.Unmarshal(runAdapterJSON(t, workspace, repo, "friction", "scan"), &firstFriction); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(runAdapterJSON(t, workspace, repo, "friction", "scan"), &secondFriction); err != nil {
		t.Fatal(err)
	}
	if firstFriction.Created != 1 || secondFriction.Created != 0 || secondFriction.Skipped != 1 {
		t.Fatalf("friction first=%+v second=%+v", firstFriction, secondFriction)
	}
	var links []MemoryLink
	if err := json.Unmarshal(runAdapterJSON(t, workspace, repo, "links", "--plan", "contract"), &links); err != nil {
		t.Fatal(err)
	}
	if len(links) != 1 || !links[0].Exists {
		t.Fatalf("links=%+v", links)
	}

	ctx, err = app.Open(repo, "", false)
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	notes, _ := architecture.List(ctx.DB, architecture.ListFilter{})
	feedbackRows, _ := feedback.List(ctx.DB, "open", 0)
	if len(notes) != 2 || len(feedbackRows) != 1 {
		t.Fatalf("notes=%d feedback=%d", len(notes), len(feedbackRows))
	}
}
