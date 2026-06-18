package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveExplicitRepo(t *testing.T) {
	dir := t.TempDir()
	proj, err := Resolve(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	abs, _ := filepath.Abs(dir)
	if proj.RepoRoot != abs {
		t.Fatalf("repo=%q want %q", proj.RepoRoot, abs)
	}
	if proj.DBPath != filepath.Join(abs, DevDBDirName, DefaultDBName) {
		t.Fatalf("db=%q", proj.DBPath)
	}
}

func TestResolveExplicitDB(t *testing.T) {
	dir := t.TempDir()
	custom := filepath.Join(dir, "custom.db")
	proj, err := Resolve(dir, custom)
	if err != nil {
		t.Fatal(err)
	}
	if proj.DBPath != custom {
		t.Fatalf("db=%q want %q", proj.DBPath, custom)
	}
}

func TestResolveDiscoversDevDB(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub", "nested")
	if err := os.MkdirAll(filepath.Join(dir, DevDBDirName), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(sub); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	proj, err := Resolve("", "")
	if err != nil {
		t.Fatal(err)
	}
	abs, _ := filepath.Abs(dir)
	if proj.RepoRoot != abs {
		t.Fatalf("repo=%q want %q", proj.RepoRoot, abs)
	}
}

func TestResolveDiscoversGit(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "pkg")
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(sub); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	proj, err := Resolve("", "")
	if err != nil {
		t.Fatal(err)
	}
	abs, _ := filepath.Abs(dir)
	if proj.RepoRoot != abs {
		t.Fatalf("repo=%q want %q", proj.RepoRoot, abs)
	}
}
