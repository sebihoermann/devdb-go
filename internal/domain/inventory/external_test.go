package inventory_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/domain/inventory"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestSyncExternalSourcesIsNamespacedAndIdempotent(t *testing.T) {
	db, _ := testutil.TempDB(t)
	sources := []inventory.ExternalSource{{Path: "MEMORY.md", Language: "markdown", Lines: 2, ContentHash: "h1", SizeBytes: 12}}
	first, err := inventory.SyncExternalSources(db, "openclaw-test", sources, "test")
	if err != nil {
		t.Fatal(err)
	}
	if first.FilesAdded != 1 || first.Paths["MEMORY.md"] != "external/openclaw-test/MEMORY.md" {
		t.Fatalf("first=%+v", first)
	}
	second, err := inventory.SyncExternalSources(db, "openclaw-test", sources, "test")
	if err != nil {
		t.Fatal(err)
	}
	if second.FilesAdded != 0 || second.FilesChanged != 0 || second.FilesRemoved != 0 {
		t.Fatalf("second=%+v", second)
	}
	sources[0].ContentHash = "h2"
	changed, err := inventory.SyncExternalSources(db, "openclaw-test", sources, "test")
	if err != nil || changed.FilesChanged != 1 {
		t.Fatalf("changed=%+v err=%v", changed, err)
	}
	removed, err := inventory.SyncExternalSources(db, "openclaw-test", nil, "test")
	if err != nil || removed.FilesRemoved != 1 {
		t.Fatalf("removed=%+v err=%v", removed, err)
	}
}

func TestRepositoryScanPreservesExternalSources(t *testing.T) {
	db, _ := testutil.TempDB(t)
	if _, err := inventory.SyncExternalSources(db, "openclaw-test", []inventory.ExternalSource{{Path: "MEMORY.md", ContentHash: "h1"}}, "test"); err != nil {
		t.Fatal(err)
	}
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := inventory.Scan(db, repo, nil, false, "test"); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM repo_files WHERE kind='external'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("external rows=%d", count)
	}
}

func TestSyncExternalSourcesRejectsTraversal(t *testing.T) {
	db, _ := testutil.TempDB(t)
	if _, err := inventory.SyncExternalSources(db, "openclaw-test", []inventory.ExternalSource{{Path: "../secret"}}, "test"); err == nil {
		t.Fatal("expected traversal error")
	}
}
