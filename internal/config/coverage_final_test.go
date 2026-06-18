package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveFallbackCwd(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "deep", "nested")
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
	absSub, _ := filepath.Abs(sub)
	if proj.RepoRoot != absSub {
		t.Fatalf("repo=%q want %q", proj.RepoRoot, absSub)
	}
}

func TestResolveRepoFlagAbs(t *testing.T) {
	dir := t.TempDir()
	rel := "."
	proj, err := Resolve(rel, "")
	if err != nil {
		t.Fatal(err)
	}
	abs, _ := filepath.Abs(rel)
	if proj.RepoRoot != abs {
		t.Fatalf("abs repo=%q", proj.RepoRoot)
	}
	customDB := filepath.Join(dir, "x.db")
	proj, err = Resolve(dir, customDB)
	if err != nil || proj.DBPath != customDB {
		t.Fatalf("custom db=%q err=%v", proj.DBPath, err)
	}
}
