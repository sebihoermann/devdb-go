package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestTempDBAndGrassFixture(t *testing.T) {
	db, path := TempDB(t)
	if db == nil || path == "" {
		t.Fatal("expected db and path")
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM feedback`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	fixture := GrassFixture(t, "dead_code.py")
	if _, err := os.Stat(fixture); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	root := RepoRoot(t)
	if _, err := os.Stat(root + "/go.mod"); err != nil {
		t.Fatalf("repo root: %v", err)
	}
}

func TestInitGitRepo(t *testing.T) {
	dir := t.TempDir()
	InitGitRepo(t, dir)

	gitDir := filepath.Join(dir, ".git")
	if st, err := os.Stat(gitDir); err != nil || !st.IsDir() {
		t.Fatalf(".git missing: %v", err)
	}
	readme := filepath.Join(dir, "README.md")
	if _, err := os.Stat(readme); err != nil {
		t.Fatalf("README missing: %v", err)
	}

	cmd := exec.Command("git", "-C", dir, "log", "-1", "--oneline")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git log: %v\n%s", err, out)
	}
	if len(out) == 0 {
		t.Fatal("expected commit history")
	}

	status := exec.Command("git", "-C", dir, "status", "--porcelain")
	statusOut, err := status.CombinedOutput()
	if err != nil {
		t.Fatalf("git status: %v", err)
	}
	if len(statusOut) != 0 {
		t.Fatalf("expected clean tree, got %q", statusOut)
	}
}

func TestRepoRootWalksUpward(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := RepoRoot(t)
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("go.mod under root: %v", err)
	}
	if !filepath.IsAbs(root) {
		t.Fatalf("root should be absolute: %s", root)
	}
	if cwd == "" {
		t.Fatal("unexpected empty cwd")
	}
}

func TestTempDBCreatesMigratedSchema(t *testing.T) {
	db, path := TempDB(t)
	for _, table := range []string{"feedback", "plan_items", "goals", "schema_migrations"} {
		var name string
		err := db.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table,
		).Scan(&name)
		if err != nil {
			t.Fatalf("table %s: %v", table, err)
		}
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

func TestRepoRootFromSubdirectory(t *testing.T) {
	root := RepoRoot(t)
	sub := filepath.Join(root, "internal", "testutil")
	if err := os.Chdir(sub); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(root) })
	if got := RepoRoot(t); got != root {
		t.Fatalf("RepoRoot from subdir: got %s want %s", got, root)
	}
}

func TestInitGitRepoIdempotent(t *testing.T) {
	dir := t.TempDir()
	InitGitRepo(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "extra.txt"), []byte("more\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "-C", dir, "add", "extra.txt")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "-C", dir, "-c", "user.email=test@test.com", "-c", "user.name=Test", "commit", "-m", "second")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}
}

func TestGrassFixtureUsesArchiveFallbackPath(t *testing.T) {
	modDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(modDir, "go.mod"), []byte("module grass.test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	parent := filepath.Dir(modDir)
	archiveDir := filepath.Join(parent, "archive", "python", "tests", "fixtures", "grass_fixtures")
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		t.Fatal(err)
	}
	proxy := filepath.Join(archiveDir, "proxy.py")
	if err := os.WriteFile(proxy, []byte("pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	internal := filepath.Join(modDir, "internal", "pkg")
	if err := os.MkdirAll(internal, 0o755); err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(internal); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	got := GrassFixture(t, "proxy.py")
	if got != proxy {
		t.Fatalf("fixture path: got %s want %s", got, proxy)
	}
}

func TestGrassFixtureFromArchiveFallback(t *testing.T) {
	fixture := GrassFixture(t, "dead_code.py")
	if filepath.Base(fixture) != "dead_code.py" {
		t.Fatalf("fixture path: %s", fixture)
	}
}
