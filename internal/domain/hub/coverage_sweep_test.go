package hub

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/domain/feedback"
	"github.com/sebihoermann/devdb-go/internal/domain/planning"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestProjectDetailWithAttention(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	dbPath := filepath.Join(dir, ".devdb", "development.db")
	registry := filepath.Join(dir, "registry.json")
	metaDB := filepath.Join(dir, "metadata.db")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	srcDB, srcPath := testutil.TempDB(t)
	planID, _ := planning.CreatePlan(srcDB, planning.CreatePlanInput{Slug: "hub", Title: "Hub", ModelID: "test"})
	msID, _ := planning.AddMilestone(srcDB, planID, "M1", "", "test", 1)
	itemID, _ := planning.AddStructuredItem(srcDB, planning.StructuredItemInput{
		PlanID: planID, MilestoneID: msID, Title: "In flight", ModelID: "test",
	})
	_, _ = planning.StartItem(srcDB, itemID, "test")
	_, _ = feedback.Add(srcDB, feedback.AddInput{Role: "model", Note: "open issue", ModelID: "test"})
	_ = srcDB.Close()
	data, _ := os.ReadFile(srcPath)
	_ = os.MkdirAll(filepath.Dir(dbPath), 0o755)
	if err := os.WriteFile(dbPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	reg, err := Register(repo, "attn", registry, metaDB)
	if err != nil {
		t.Fatal(err)
	}
	hubDB, err := OpenHub(metaDB)
	if err != nil {
		t.Fatal(err)
	}
	defer hubDB.Close()
	if _, err := SyncAll(registry, metaDB, false); err != nil {
		t.Fatal(err)
	}
	detail, err := Project(registry, metaDB, reg.Alias)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Snapshot.InFlightTitle == "" && len(detail.Attention) == 0 {
		t.Fatalf("detail=%+v", detail)
	}
	rows, err := Dashboard(registry, metaDB, "quality", false)
	if err != nil || len(rows) == 0 {
		t.Fatalf("dashboard=%d err=%v", len(rows), err)
	}
	entries, err := List(registry, metaDB, true)
	if err != nil || len(entries) == 0 {
		t.Fatalf("list=%d err=%v", len(entries), err)
	}
}

func TestDoctorAndMarkDirtyPaths(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	dbPath := filepath.Join(dir, ".devdb", "development.db")
	registry := filepath.Join(dir, "registry.json")
	metaDB := filepath.Join(dir, "metadata.db")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	srcDB, srcPath := testutil.TempDB(t)
	_ = srcDB.Close()
	data, _ := os.ReadFile(srcPath)
	_ = os.MkdirAll(filepath.Dir(dbPath), 0o755)
	_ = os.WriteFile(dbPath, data, 0o644)
	reg, err := Register(repo, "doc", registry, metaDB)
	if err != nil {
		t.Fatal(err)
	}
	MarkDirty(dbPath)
	rows, err := Doctor(registry, metaDB, reg.Alias)
	if err != nil || len(rows) == 0 {
		t.Fatalf("doctor=%d err=%v", len(rows), err)
	}
	allDoctor, err := Doctor(registry, metaDB, "")
	if err != nil || len(allDoctor) == 0 {
		t.Fatalf("all doctor=%d err=%v", len(allDoctor), err)
	}
}
