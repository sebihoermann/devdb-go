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

	registered, err := hub.AutoRegister(dir, registry, metaDB, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(registered.Registered) != 1 || registered.Registered[0] != "myproj" {
		t.Fatalf("expected [myproj], got %v", registered.Registered)
	}
}

func TestAutoRegisterMultipleProjects(t *testing.T) {
	dir := t.TempDir()
	registry := filepath.Join(dir, "registry")
	metaDB := filepath.Join(dir, "metadata.db")
	for _, name := range []string{"alpha", "beta", "gamma"} {
		touchDevdb(t, filepath.Join(dir, name))
	}
	registered, err := hub.AutoRegister(dir, registry, metaDB, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(registered.Registered) != 3 {
		t.Fatalf("expected 3, got %v", registered.Registered)
	}
	want := map[string]bool{"alpha": true, "beta": true, "gamma": true}
	for _, a := range registered.Registered {
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
	registered, err := hub.AutoRegister(dir, registry, metaDB, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(registered.Registered) != 1 || registered.Registered[0] != "real" {
		t.Fatalf("expected [real], got %v", registered.Registered)
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
	registered, err := hub.AutoRegister(dir, registry, metaDB, false)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, a := range registered.Registered {
		got[a] = true
	}
	if !got["outer"] || !got["sub"] {
		t.Fatalf("expected both outer and sub registered, got %v", registered.Registered)
	}
}

func TestAutoRegisterEmptyScope(t *testing.T) {
	dir := t.TempDir()
	registry := filepath.Join(dir, "registry")
	metaDB := filepath.Join(dir, "metadata.db")
	registered, err := hub.AutoRegister(dir, registry, metaDB, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(registered.Registered) != 0 {
		t.Fatalf("expected empty, got %v", registered.Registered)
	}
}

// TestAutoRegisterDryRunWritesNothing is acceptance 1: the count matches
// FindUnregistered and no registry exists afterwards.
func TestAutoRegisterDryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	registry := filepath.Join(dir, "registry")
	metaDB := filepath.Join(dir, "metadata.db")
	for _, name := range []string{"one", "two"} {
		touchDevdb(t, filepath.Join(dir, name))
	}

	unregistered, err := hub.FindUnregistered([]string{dir}, registry)
	if err != nil {
		t.Fatal(err)
	}

	res, err := hub.AutoRegister(dir, registry, metaDB, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Registered) != len(unregistered) {
		t.Fatalf("dry-run would register %d, doctor --scan sees %d unregistered",
			len(res.Registered), len(unregistered))
	}
	if _, err := os.Stat(registry); !os.IsNotExist(err) {
		t.Fatalf("dry-run created the registry: %v", err)
	}
	if _, err := os.Stat(metaDB); !os.IsNotExist(err) {
		t.Fatalf("dry-run created the metadata db: %v", err)
	}
}

// TestAutoRegisterLeavesNothingUnregistered is acceptance 2.
func TestAutoRegisterLeavesNothingUnregistered(t *testing.T) {
	dir := t.TempDir()
	registry := filepath.Join(dir, "registry")
	metaDB := filepath.Join(dir, "metadata.db")
	for _, name := range []string{"one", "two", "three"} {
		touchDevdb(t, filepath.Join(dir, name))
	}
	if _, err := hub.AutoRegister(dir, registry, metaDB, false); err != nil {
		t.Fatal(err)
	}
	left, err := hub.FindUnregistered([]string{dir}, registry)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 0 {
		t.Fatalf("still unregistered: %+v", left)
	}
}

// TestAutoRegisterIsIdempotent is acceptance 3's first half: a second pass adds
// nothing and rewrites no aliases.
func TestAutoRegisterIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	registry := filepath.Join(dir, "registry")
	metaDB := filepath.Join(dir, "metadata.db")
	for _, name := range []string{"alpha", "beta"} {
		touchDevdb(t, filepath.Join(dir, name))
	}
	first, err := hub.AutoRegister(dir, registry, metaDB, false)
	if err != nil {
		t.Fatal(err)
	}
	before, err := hub.ReadRegistry(registry)
	if err != nil {
		t.Fatal(err)
	}

	second, err := hub.AutoRegister(dir, registry, metaDB, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Registered) != 0 {
		t.Fatalf("second pass registered %v", second.Registered)
	}

	after, err := hub.ReadRegistry(registry)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) {
		t.Fatalf("registry size changed %d -> %d", len(before), len(after))
	}
	for i := range before {
		if before[i].Alias != after[i].Alias || before[i].Root != after[i].Root {
			t.Fatalf("entry %d changed: %+v -> %+v", i, before[i], after[i])
		}
	}
	if len(first.Registered) != 2 {
		t.Fatalf("first pass registered %v", first.Registered)
	}
}

// TestAutoRegisterNeverRepointsExistingAlias is acceptance 3's second half:
// two checkouts with the same directory name must not fight over the alias.
func TestAutoRegisterNeverRepointsExistingAlias(t *testing.T) {
	dir := t.TempDir()
	registry := filepath.Join(dir, "registry")
	metaDB := filepath.Join(dir, "metadata.db")
	first := filepath.Join(dir, "a", "proj")
	second := filepath.Join(dir, "b", "proj")
	touchDevdb(t, first)

	res, err := hub.AutoRegister(dir, registry, metaDB, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Registered) != 1 || res.Registered[0] != "proj" {
		t.Fatalf("first pass = %v", res.Registered)
	}

	// Now a second checkout with the same basename appears.
	touchDevdb(t, second)
	res, err = hub.AutoRegister(dir, registry, metaDB, false)
	if err != nil {
		t.Fatal(err)
	}

	projects, err := hub.ReadRegistry(registry)
	if err != nil {
		t.Fatal(err)
	}
	var aliasForFirst string
	aliases := map[string]string{}
	for _, p := range projects {
		aliases[p.Alias] = p.Root
		if p.Root == first {
			aliasForFirst = p.Alias
		}
	}
	if aliasForFirst != "proj" {
		t.Fatalf("existing alias repointed: proj now resolves to %q", aliases["proj"])
	}
	if aliases["proj"] != first {
		t.Fatalf("alias proj points at %q, want %q", aliases["proj"], first)
	}
	// The newcomer is registered under a fresh alias, not dropped.
	var registeredSecond bool
	for alias, root := range aliases {
		if root == second && alias != "proj" {
			registeredSecond = true
		}
	}
	if !registeredSecond {
		t.Fatalf("second checkout not registered: %+v", aliases)
	}
	if len(res.Conflicts) != 1 || res.Conflicts[0].Alias != "proj" {
		t.Fatalf("conflict not reported: %+v", res.Conflicts)
	}
}
