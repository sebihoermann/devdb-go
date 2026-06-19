package hub_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/domain/hub"
)

func touchDevdb(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, ".devdb"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".devdb", "development.db"), []byte("placeholder"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAutoRegisterSingleProject(t *testing.T) {
	dir := t.TempDir()
	registry := filepath.Join(dir, "registry")
	metaDB := filepath.Join(dir, "metadata.db")
	root := filepath.Join(dir, "myproj")
	touchDevdb(t, root)

	registered, err := hub.AutoRegister(dir, registry, metaDB)
	if err != nil {
		t.Fatal(err)
	}
	if len(registered) != 1 || registered[0] != "myproj" {
		t.Fatalf("expected [myproj], got %v", registered)
	}
}

func TestAutoRegisterMultipleProjects(t *testing.T) {
	dir := t.TempDir()
	registry := filepath.Join(dir, "registry")
	metaDB := filepath.Join(dir, "metadata.db")
	for _, name := range []string{"alpha", "beta", "gamma"} {
		touchDevdb(t, filepath.Join(dir, name))
	}
	registered, err := hub.AutoRegister(dir, registry, metaDB)
	if err != nil {
		t.Fatal(err)
	}
	if len(registered) != 3 {
		t.Fatalf("expected 3, got %v", registered)
	}
	want := map[string]bool{"alpha": true, "beta": true, "gamma": true}
	for _, a := range registered {
		if !want[a] {
			t.Fatalf("unexpected alias %q in %v", a, registered)
		}
	}
}

func TestAutoRegisterIgnoresNonDevdbDatabases(t *testing.T) {
	dir := t.TempDir()
	registry := filepath.Join(dir, "registry")
	metaDB := filepath.Join(dir, "metadata.db")
	touchDevdb(t, filepath.Join(dir, "real"))
	if err := os.WriteFile(filepath.Join(dir, "stray.sqlite"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "nodb"), 0o755); err != nil {
		t.Fatal(err)
	}
	registered, err := hub.AutoRegister(dir, registry, metaDB)
	if err != nil {
		t.Fatal(err)
	}
	if len(registered) != 1 || registered[0] != "real" {
		t.Fatalf("expected [real], got %v", registered)
	}
}

func TestAutoRegisterFindsNestedProjects(t *testing.T) {
	dir := t.TempDir()
	registry := filepath.Join(dir, "registry")
	metaDB := filepath.Join(dir, "metadata.db")
	root := filepath.Join(dir, "outer")
	touchDevdb(t, root)
	if err := os.MkdirAll(filepath.Join(root, "sub", ".devdb"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sub", ".devdb", "development.db"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	registered, err := hub.AutoRegister(dir, registry, metaDB)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, a := range registered {
		got[a] = true
	}
	if !got["outer"] || !got["sub"] {
		t.Fatalf("expected both outer and sub registered, got %v", registered)
	}
}

func TestAutoRegisterEmptyScope(t *testing.T) {
	dir := t.TempDir()
	registry := filepath.Join(dir, "registry")
	metaDB := filepath.Join(dir, "metadata.db")
	registered, err := hub.AutoRegister(dir, registry, metaDB)
	if err != nil {
		t.Fatal(err)
	}
	if len(registered) != 0 {
		t.Fatalf("expected empty, got %v", registered)
	}
}
