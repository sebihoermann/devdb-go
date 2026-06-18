package testutil

import "testing"

func TestAllFixtureHelpers(t *testing.T) {
	root := RepoRoot(t)
	if root == "" {
		t.Fatal("empty repo root")
	}
	fixture := GrassFixture(t, "dead_code.py")
	if fixture == "" {
		t.Fatal("empty fixture path")
	}
	db, path := TempDB(t)
	if db == nil || path == "" {
		t.Fatal("temp db missing")
	}
	dir := t.TempDir()
	InitGitRepo(t, dir)
}
