package hub_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/domain/hub"
)

// makeProject creates a project root with a placeholder ledger so Register
// records it as existing.
func makeProject(t *testing.T, root string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, ".devdb"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".devdb", "development.db"), []byte("placeholder"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// TestRegisterRejectsExplicitAliasHijack is the guard for the defect where
// `hub register PATH --alias <existing>` silently repointed an alias away from
// the project that owned it. The alias protection existed only in AutoRegister,
// so the explicit verb — the one an agent reaches for — bypassed it.
func TestRegisterRejectsExplicitAliasHijack(t *testing.T) {
	dir := t.TempDir()
	registry := filepath.Join(dir, "registry")
	metaDB := filepath.Join(dir, "metadata.db")
	owner := makeProject(t, filepath.Join(dir, "owner"))
	intruder := makeProject(t, filepath.Join(dir, "intruder"))

	if _, err := hub.Register(owner, "devdb-go", registry, metaDB); err != nil {
		t.Fatal(err)
	}

	_, err := hub.Register(intruder, "devdb-go", registry, metaDB)
	if !errors.Is(err, hub.ErrAliasTaken) {
		t.Fatalf("Register to a taken alias: err=%v, want ErrAliasTaken", err)
	}

	// The alias must still describe the original root, in the registry and in
	// the hub. A refusal that left the hub row repointed would be no fix.
	entries, err := hub.List(registry, metaDB, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries=%+v, want exactly the original project", entries)
	}
	if entries[0].Alias != "devdb-go" || entries[0].Root != owner {
		t.Fatalf("alias repointed: %+v, want devdb-go -> %s", entries[0], owner)
	}
}

// TestRegisterDerivedAliasSuffixesOnCollision covers the other half of the
// same rule: a derived alias (no --alias) must not fail or overwrite either —
// it gets a stable -2 suffix, which is what AutoRegister does.
func TestRegisterDerivedAliasSuffixesOnCollision(t *testing.T) {
	dir := t.TempDir()
	registry := filepath.Join(dir, "registry")
	metaDB := filepath.Join(dir, "metadata.db")
	first := makeProject(t, filepath.Join(dir, "a", "project"))
	second := makeProject(t, filepath.Join(dir, "b", "project"))

	p1, err := hub.Register(first, "", registry, metaDB)
	if err != nil {
		t.Fatal(err)
	}
	p2, err := hub.Register(second, "", registry, metaDB)
	if err != nil {
		t.Fatal(err)
	}
	if p1.Alias != "project" {
		t.Fatalf("first alias=%q, want project", p1.Alias)
	}
	if p2.Alias != "project-2" {
		t.Fatalf("second alias=%q, want project-2", p2.Alias)
	}

	entries, err := hub.List(registry, metaDB, false)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, e := range entries {
		got[e.Alias] = e.Root
	}
	if got["project"] != first || got["project-2"] != second {
		t.Fatalf("registry=%v", got)
	}
}

// TestRegisterSameRootTwiceIsIdempotent pins the keep-working case: the same
// root and alias may be registered again without an error.
func TestRegisterSameRootTwiceIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	registry := filepath.Join(dir, "registry")
	metaDB := filepath.Join(dir, "metadata.db")
	root := makeProject(t, filepath.Join(dir, "proj"))

	for i := 0; i < 2; i++ {
		if _, err := hub.Register(root, "proj", registry, metaDB); err != nil {
			t.Fatalf("register #%d: %v", i+1, err)
		}
	}
	entries, err := hub.List(registry, metaDB, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries=%+v, want 1", entries)
	}
}

// TestAutoRegisterSuffixesInsteadOfFailing covers the interaction between the
// new explicit-alias guard and the walker: AutoRegister always derives its own
// alias, so a collision must suffix rather than surface ErrAliasTaken and drop
// the project. This pins that the guard did not make auto-registration brittle.
func TestAutoRegisterSuffixesInsteadOfFailing(t *testing.T) {
	dir := t.TempDir()
	registry := filepath.Join(dir, "registry")
	metaDB := filepath.Join(dir, "metadata.db")

	first := makeProject(t, filepath.Join(dir, "one", "dup"))
	second := makeProject(t, filepath.Join(dir, "two", "dup"))

	res, err := hub.AutoRegister(dir, registry, metaDB, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Discovered) != 2 || len(res.Registered) != 2 {
		t.Fatalf("res=%+v, want 2 discovered and 2 registered", res)
	}

	entries, err := hub.List(registry, metaDB, false)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, e := range entries {
		got[e.Alias] = e.Root
	}
	if got["dup"] != first || got["dup-2"] != second {
		t.Fatalf("registry=%v, want dup->%s and dup-2->%s", got, first, second)
	}
}

// TestRegisterRejectsAliasHeldOnlyByHub covers the split-store case: the hub
// still lists an alias the registry has already dropped (or never had). A
// registry-only check would let the guarded upsert silently do nothing, leaving
// the two stores pointing at different projects.
func TestRegisterRejectsAliasHeldOnlyByHub(t *testing.T) {
	dir := t.TempDir()
	registry := filepath.Join(dir, "registry")
	metaDB := filepath.Join(dir, "metadata.db")
	owner := makeProject(t, filepath.Join(dir, "owner"))
	intruder := makeProject(t, filepath.Join(dir, "intruder"))

	if _, err := hub.Register(owner, "shared", registry, metaDB); err != nil {
		t.Fatal(err)
	}
	// Drop it from the registry only, as a stale/partial state would.
	if err := hub.WriteRegistry(registry, nil); err != nil {
		t.Fatal(err)
	}

	if _, err := hub.Register(intruder, "shared", registry, metaDB); !errors.Is(err, hub.ErrAliasTaken) {
		t.Fatalf("err=%v, want ErrAliasTaken for a hub-owned alias", err)
	}

	hubDB, err := hub.OpenHub(metaDB)
	if err != nil {
		t.Fatal(err)
	}
	defer hubDB.Close()
	var got string
	if err := hubDB.QueryRow(`SELECT root_path FROM projects WHERE alias='shared'`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != owner {
		t.Fatalf("hub alias shared -> %s, want %s", got, owner)
	}
}
