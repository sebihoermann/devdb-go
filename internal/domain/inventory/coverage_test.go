package inventory_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sebihoermann/devdb-go/internal/domain/architecture"
	"github.com/sebihoermann/devdb-go/internal/domain/inventory"
	"github.com/sebihoermann/devdb-go/internal/domain/reminders"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestGitignoreMatching(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte("build/\n*.log\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "build"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "build", "out.bin"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "app.log"), []byte("log"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	found, err := inventory.DiscoverFiles(repo, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, p := range found {
		seen[p] = true
	}
	if seen["build/out.bin"] || seen["app.log"] {
		t.Fatalf("gitignored files discovered: %v", found)
	}
	if !seen["main.go"] {
		t.Fatal("main.go should be discovered")
	}
}

func TestScanScopedPathsAndRemoval(t *testing.T) {
	db, _ := testutil.TempDB(t)
	repo := t.TempDir()
	for _, rel := range []string{"pkg/a.go", "other/b.go"} {
		p := filepath.Join(repo, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := inventory.Scan(db, repo, []string{"pkg"}, false, "test"); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM repo_files`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("scoped scan indexed %d files", n)
	}
}

func TestFormatContextHumanLongBody(t *testing.T) {
	db, _ := testutil.TempDB(t)
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "x.go"), []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := inventory.Scan(db, repo, nil, false, "test"); err != nil {
		t.Fatal(err)
	}
	body := strings.Join([]string{"line1", "line2", "line3", "line4"}, "\n")
	if _, err := architecture.Add(db, "long-body", body, []string{"x.go"}, "high", "test"); err != nil {
		t.Fatal(err)
	}
	payload, err := inventory.Context(db, inventory.ContextOptions{Files: []string{"x.go"}, Task: "edit"})
	if err != nil {
		t.Fatal(err)
	}
	lines := inventory.FormatContextHuman(payload)
	text := strings.Join(lines, "\n")
	if !strings.Contains(text, "line1") || !strings.Contains(text, "...") {
		t.Fatalf("expected truncated body in output: %s", text)
	}
}

func TestContextPlanRemindersAndOverdue(t *testing.T) {
	db, _ := testutil.TempDB(t)
	if _, err := db.Exec(`INSERT INTO plan_items(id, title, status, created_at, model_id) VALUES ('pi1','task','planned',datetime('now'),'test')`); err != nil {
		t.Fatal(err)
	}
	past := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339Nano)
	if _, err := reminders.Add(db, reminders.AddInput{
		Title: "due soon", PlanItemID: "pi1", DueAt: past, ModelID: "test",
	}); err != nil {
		t.Fatal(err)
	}
	payload, err := inventory.Context(db, inventory.ContextOptions{PlanItemID: "pi1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(payload.PlanReminders) != 1 {
		t.Fatalf("plan reminders=%d", len(payload.PlanReminders))
	}
	lines := inventory.FormatContextHuman(payload)
	if !strings.Contains(strings.Join(lines, "\n"), "OVERDUE") {
		t.Fatal("expected overdue marker")
	}
}

func TestDiscoverSingleFilePath(t *testing.T) {
	repo := t.TempDir()
	p := filepath.Join(repo, "solo.go")
	if err := os.WriteFile(p, []byte("package solo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	found, err := inventory.DiscoverFiles(repo, []string{"solo.go"}, false)
	if err != nil || len(found) != 1 || found[0] != "solo.go" {
		t.Fatalf("found=%v err=%v", found, err)
	}
}
