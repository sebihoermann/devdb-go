package inventory_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/domain/inventory"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestDiscoverFilesWalkMode(t *testing.T) {
	repo := t.TempDir()
	files := map[string]string{
		"pkg/main.go":          "package main\n",
		"tests/test_foo.py":    "def test_x(): pass\n",
		"README.md":            "# readme\n",
		"CLAUDE.md":            "# agent\n",
		"pyproject.toml":       "[tool]\n",
		".gitignore":           "node_modules/\n",
		"node_modules/x.js":    "x\n", // ignored dir
	}
	for rel, content := range files {
		p := filepath.Join(repo, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	found, err := inventory.DiscoverFiles(repo, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, p := range found {
		seen[p] = true
	}
	for _, want := range []string{"pkg/main.go", "tests/test_foo.py", "README.md", "CLAUDE.md", "pyproject.toml", ".gitignore"} {
		if !seen[want] {
			t.Fatalf("missing %q in %v", want, found)
		}
	}
	if seen["node_modules/x.js"] {
		t.Fatal("should ignore node_modules")
	}
}

func TestDiscoverFilesGitAware(t *testing.T) {
	repo := t.TempDir()
	testutil.InitGitRepo(t, repo)
	src := filepath.Join(repo, "tracked.go")
	if err := os.WriteFile(src, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// commit tracked file
	runGit(t, repo, "add", "tracked.go")
	runGit(t, repo, "commit", "-m", "add tracked")

	untracked := filepath.Join(repo, "untracked.go")
	if err := os.WriteFile(untracked, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	found, err := inventory.DiscoverFiles(repo, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, p := range found {
		seen[p] = true
	}
	if !seen["tracked.go"] {
		t.Fatalf("expected tracked.go in %v", found)
	}
	if seen["untracked.go"] {
		t.Fatal("git-aware should not list untracked files")
	}
}

func TestScanInventoryKinds(t *testing.T) {
	repo := t.TempDir()
	fixtures := []struct {
		path, wantKind, wantLang string
	}{
		{"main.go", "code", "go"},
		{"tests/test_a.py", "test", "python"},
		{"docs/guide.md", "doc", "markdown"},
		{"CLAUDE.md", "agent_doc", "markdown"},
		{"config.toml", "config", "toml"},
		{"data.bin", "binary", ""},
	}
	for _, f := range fixtures {
		p := filepath.Join(repo, f.path)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		data := []byte("content\n")
		if f.wantKind == "binary" {
			data = []byte{0x00, 0x01, 0x02}
		}
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	records, err := inventory.ScanInventory(repo, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	byPath := map[string]inventory.FileRecord{}
	for _, r := range records {
		byPath[r.Path] = r
	}
	for _, f := range fixtures {
		rec, ok := byPath[f.path]
		if !ok {
			t.Fatalf("missing record for %s", f.path)
		}
		if rec.Kind != f.wantKind {
			t.Fatalf("%s kind=%q want %q", f.path, rec.Kind, f.wantKind)
		}
		if f.wantLang != "" && rec.Language != f.wantLang {
			t.Fatalf("%s lang=%q want %q", f.path, rec.Language, f.wantLang)
		}
	}
}

func TestScanFileSkipsTransient(t *testing.T) {
	repo := t.TempDir()
	p := filepath.Join(repo, "foo.tmp")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec, err := inventory.ScanFile(repo, "foo.tmp")
	if err != nil {
		t.Fatal(err)
	}
	if rec != nil {
		t.Fatalf("expected nil for transient, got %+v", rec)
	}
}

func TestDiscoverFilesScopedPath(t *testing.T) {
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
	found, err := inventory.DiscoverFiles(repo, []string{"pkg"}, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range found {
		if p == "other/b.go" {
			t.Fatalf("scoped discover included %s", p)
		}
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
