package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRepoRootAndGrassFixture(t *testing.T) {
	root := RepoRoot(t)
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("go.mod missing under %s", root)
	}
	fixture := GrassFixture(t, "lazy_user.py")
	if fixture == "" {
		t.Fatal("empty fixture path")
	}
	if _, err := os.Stat(fixture); err != nil {
		t.Fatalf("fixture: %v", err)
	}
}

func TestTempDBAndInitGitRepo(t *testing.T) {
	db, path := TempDB(t)
	if db == nil || path == "" {
		t.Fatal("temp db failed")
	}
	_ = db.Close()
	dir := t.TempDir()
	InitGitRepo(t, dir)
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		t.Fatal("git not initialized")
	}
}
