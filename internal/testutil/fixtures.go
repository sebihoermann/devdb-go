package testutil

import (
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/migrate"
	"github.com/sebihoermann/devdb-go/internal/storage"
)

// RepoRoot returns the golang module directory (contains go.mod).
func RepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

// LegacyPythonDBPath returns a Python-schema ledger in the repo for integration tests, or "" if none.
func LegacyPythonDBPath(t *testing.T) string {
	t.Helper()
	root := RepoRoot(t)
	candidates := []string{
		filepath.Join(root, ".devdb", "development.db.python-bak"),
		filepath.Join(root, ".devdb", "development.db"),
		filepath.Join(filepath.Dir(root), ".devdb", "development.db.python-bak"),
		filepath.Join(filepath.Dir(root), ".devdb", "development.db"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err != nil {
			continue
		}
		db, err := storage.Open(p)
		if err != nil {
			continue
		}
		kind, _, err := storage.DetectSchema(db)
		_ = db.Close()
		if err == nil && kind == storage.SchemaPython {
			return p
		}
	}
	return ""
}

// GrassFixture returns an absolute path to tests/fixtures/grass_fixtures/name.
func GrassFixture(t *testing.T, name string) string {
	t.Helper()
	root := RepoRoot(t)
	parent := filepath.Dir(root)
	candidates := []string{
		filepath.Join(root, "tests", "fixtures", "grass_fixtures", name),
		filepath.Join(parent, "tests", "fixtures", "grass_fixtures", name),
		filepath.Join(parent, "archive", "python", "tests", "fixtures", "grass_fixtures", name),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	t.Fatalf("grass fixture %q not found under %v", name, candidates)
	return ""
}

// InitGitRepo initializes a git repository with one commit.
func InitGitRepo(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test User"},
	} {
		cmd := exec.Command(args[0], append(args[1:], dir)...)
		if args[0] == "git" && args[1] == "init" {
			cmd = exec.Command("git", "init", dir)
		} else if args[1] == "config" {
			cmd = exec.Command("git", "-C", dir, "config", args[2], args[3])
		}
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	readme := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readme, []byte("init\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "-C", dir, "add", ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "-C", dir, "commit", "-m", "init")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}
}

// TempDB creates a migrated database in t.TempDir() and returns db, path.
func TempDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "development.db")
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := migrate.RunAll(db); err != nil {
		t.Fatal(err)
	}
	return db, path
}
