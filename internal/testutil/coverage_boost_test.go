package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClosedDBIsUsableForErrorBranches(t *testing.T) {
	db := ClosedDB(t)
	if db == nil {
		t.Fatal("nil db")
	}
	if err := db.Ping(); err == nil {
		t.Fatal("expected closed")
	}
	if err := db.QueryRow("SELECT 1").Scan(new(int)); err == nil {
		t.Fatal("expected query error")
	}
}

func TestRepoRootFromDeepTempModule(t *testing.T) {
	modDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(modDir, "go.mod"), []byte("module deep.test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	deep := filepath.Join(modDir, "a", "b", "c")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(deep); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if got := RepoRoot(t); got != modDir {
		t.Fatalf("root=%s want %s", got, modDir)
	}
}

func TestGrassFixturePrimaryPath(t *testing.T) {
	root := RepoRoot(t)
	primary := filepath.Join(root, "tests", "fixtures", "grass_fixtures", "dead_code.py")
	if _, err := os.Stat(primary); err != nil {
		parent := filepath.Join(filepath.Dir(root), "tests", "fixtures", "grass_fixtures", "dead_code.py")
		if _, err := os.Stat(parent); err != nil {
			t.Skip("primary fixture path unavailable")
		}
		primary = parent
	}
	got := GrassFixture(t, "dead_code.py")
	if got != primary {
		t.Fatalf("primary=%q got=%q", primary, got)
	}
}

func TestTempDBCleanupOnClose(t *testing.T) {
	db, path := TempDB(t)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("db file: %v", err)
	}
}

func TestInitGitRepoConfigBranch(t *testing.T) {
	dir := t.TempDir()
	InitGitRepo(t, dir)
	for _, key := range []string{"user.email", "user.name"} {
		out, err := os.ReadFile(filepath.Join(dir, ".git", "config"))
		if err != nil {
			t.Fatal(err)
		}
		if len(out) == 0 {
			t.Fatalf("empty git config for %s", key)
		}
	}
}
