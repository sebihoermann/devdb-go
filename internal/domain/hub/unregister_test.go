package hub_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/domain/hub"
	"github.com/sebihoermann/devdb-go/internal/migrate"
	"github.com/sebihoermann/devdb-go/internal/storage"
)

func makeHubTestProject(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, ".devdb"), 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := storage.Open(filepath.Join(root, ".devdb", "development.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := migrate.RunAll(db); err != nil {
		t.Fatal(err)
	}
	db.Close()
}

func TestUnregisterRemovesAlias(t *testing.T) {
	dir := t.TempDir()
	registry := filepath.Join(dir, "registry")
	metaDB := filepath.Join(dir, "metadata.db")
	rootA := filepath.Join(dir, "proj-a")
	rootB := filepath.Join(dir, "proj-b")
	makeHubTestProject(t, rootA)
	makeHubTestProject(t, rootB)
	if _, err := hub.Register(rootA, "proj-a", registry, metaDB); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.Register(rootB, "proj-b", registry, metaDB); err != nil {
		t.Fatal(err)
	}
	if err := hub.Unregister("proj-a", registry, metaDB); err != nil {
		t.Fatal(err)
	}
	entries, err := hub.List(registry, metaDB, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Alias == "proj-a" {
			t.Fatalf("proj-a still present: %+v", entries)
		}
	}
	if len(entries) != 1 || entries[0].Alias != "proj-b" {
		t.Fatalf("expected only proj-b, got %+v", entries)
	}
}

func TestUnregisterUnknownAlias(t *testing.T) {
	dir := t.TempDir()
	registry := filepath.Join(dir, "registry")
	metaDB := filepath.Join(dir, "metadata.db")
	err := hub.Unregister("nonexistent", registry, metaDB)
	if !errors.Is(err, hub.ErrAliasNotFound) {
		t.Fatalf("expected ErrAliasNotFound, got %v", err)
	}
}

func TestUnregisterByRootPath(t *testing.T) {
	dir := t.TempDir()
	registry := filepath.Join(dir, "registry")
	metaDB := filepath.Join(dir, "metadata.db")
	root := filepath.Join(dir, "proj")
	makeHubTestProject(t, root)
	if _, err := hub.Register(root, "proj", registry, metaDB); err != nil {
		t.Fatal(err)
	}
	if err := hub.Unregister(root, registry, metaDB); err != nil {
		t.Fatalf("unregister by root: %v", err)
	}
	entries, _ := hub.List(registry, metaDB, false)
	if len(entries) != 0 {
		t.Fatalf("expected empty, got %+v", entries)
	}
}

func TestUnregisterLeavesErrorMessage(t *testing.T) {
	dir := t.TempDir()
	registry := filepath.Join(dir, "registry")
	metaDB := filepath.Join(dir, "metadata.db")
	err := hub.Unregister("nope", registry, metaDB)
	if err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("expected error mentioning alias, got %v", err)
	}
}
