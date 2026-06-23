package openclaw

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/app"
	"github.com/sebihoermann/devdb-go/internal/domain/planning"
)

func TestListMemoryLinksJoinsPlanItems(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "MEMORY.md"), []byte("# Memory\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	repo := t.TempDir()
	ctx, err := app.Open(repo, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := ctx.InitDB(); err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	planID, _ := planning.CreatePlan(ctx.DB, planning.CreatePlanInput{Slug: "linked", Title: "Linked", ModelID: "test"})
	if _, err := planning.AddItem(ctx.DB, planning.AddItemInput{PlanID: planID, Title: "Remember", MemoryRef: "openclaw:MEMORY.md#decision", ModelID: "test"}); err != nil {
		t.Fatal(err)
	}
	if _, err := planning.AddItem(ctx.DB, planning.AddItemInput{PlanID: planID, Title: "No ref", ModelID: "test"}); err != nil {
		t.Fatal(err)
	}
	runtime := &runtime{config: Config{Workspace: workspace, Repo: repo}, app: ctx}
	links, err := ListMemoryLinks(runtime, "linked")
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 1 || links[0].Path != "MEMORY.md" || links[0].Fragment != "decision" || !links[0].Exists {
		t.Fatalf("links=%+v", links)
	}
}

func TestListMemoryLinksRejectsTraversalForExistenceCheck(t *testing.T) {
	if safeMemoryPath("../secret") {
		t.Fatal("traversal path accepted")
	}
}
