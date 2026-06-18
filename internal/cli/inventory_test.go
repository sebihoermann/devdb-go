package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestInventoryArchSession(t *testing.T) {
	bin := buildDevdb(t)
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	dbPath := filepath.Join(dir, ".devdb", "development.db")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(repo, "main.go")
	if err := os.WriteFile(src, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	run := func(args ...string) string {
		t.Helper()
		all := append([]string{"--repo", repo, "--db", dbPath}, args...)
		stdout, stderr, code := runDevdb(t, bin, all...)
		if code != 0 {
			t.Fatalf("%v exit %d stderr=%s stdout=%s", args, code, stderr, stdout)
		}
		return firstLine(stdout)
	}

	run("init")
	runID := run("inventory", "scan")
	if len(runID) != 32 {
		t.Fatalf("scan run id: %q", runID)
	}

	noteID := run("arch", "add", "entry-main", "--body", "Main package entry.", "--source", "main.go")
	if len(noteID) != 32 {
		t.Fatalf("note id: %q", noteID)
	}

	stdout, _, code := runDevdb(t, bin, "--repo", repo, "--db", dbPath, "arch", "verify", noteID)
	if code != 0 || strings.TrimSpace(stdout) != "ok" {
		t.Fatalf("verify: code=%d out=%q", code, stdout)
	}

	ctxOut, _, code := runDevdb(t, bin, "--repo", repo, "--db", dbPath, "inventory", "context", "--files", "main.go")
	if code != 0 || !strings.Contains(ctxOut, "entry-main") {
		t.Fatalf("context: code=%d out=%q", code, ctxOut)
	}

	if err := os.WriteFile(src, []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("inventory", "scan")

	stdout, _, code = runDevdb(t, bin, "--repo", repo, "--db", dbPath, "arch", "verify", noteID)
	if code != 0 || strings.TrimSpace(stdout) != "stale" {
		t.Fatalf("verify stale: code=%d out=%q", code, stdout)
	}

	qualOut, _, code := runDevdb(t, bin, "--repo", repo, "--db", dbPath, "quality")
	if code != 0 || !strings.Contains(qualOut, "stale arch notes: 1") {
		t.Fatalf("quality: %q", qualOut)
	}
}

func TestInventorySuggestCutsDryRun(t *testing.T) {
	bin := buildDevdb(t)
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	dbPath := filepath.Join(dir, ".devdb", "development.db")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	fixture := testutil.GrassFixture(t, "dead_code.py")
	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "dead_code.py"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, cmd := range [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test User"},
		{"git", "add", "."},
		{"git", "commit", "-m", "init"},
	} {
		c := exec.Command(cmd[0], cmd[1:]...)
		c.Dir = repo
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
	}

	initArgs := []string{"--repo", repo, "--db", dbPath, "init"}
	if _, _, code := runDevdb(t, bin, initArgs...); code != 0 {
		t.Fatalf("init exit %d", code)
	}
	scanArgs := []string{"--repo", repo, "--db", dbPath, "inventory", "scan"}
	if _, _, code := runDevdb(t, bin, scanArgs...); code != 0 {
		t.Fatalf("scan exit %d", code)
	}

	stdout, stderr, code := runDevdb(t, bin, "--repo", repo, "--db", dbPath,
		"inventory", "suggest-cuts", "--dry-run", "--principles", "dead")
	if code != 0 {
		t.Fatalf("suggest-cuts exit %d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if !strings.Contains(stdout, "dead") || !strings.Contains(stdout, "dead_function") {
		t.Fatalf("stdout=%q", stdout)
	}
	if !strings.Contains(stderr, "preview only") {
		t.Fatalf("stderr=%q", stderr)
	}

	jsonOut, _, code := runDevdb(t, bin, "--json", "--repo", repo, "--db", dbPath,
		"inventory", "suggest-cuts", "--dry-run", "--principles", "dead")
	if code != 0 || !strings.Contains(jsonOut, `"persisted": false`) {
		t.Fatalf("json dry-run: code=%d out=%s", code, jsonOut)
	}

	persistStdout, persistStderr, code := runDevdb(t, bin, "--repo", repo, "--db", dbPath,
		"inventory", "suggest-cuts", "--principles", "dead")
	if code != 0 {
		t.Fatalf("persist exit %d stderr=%s", code, persistStderr)
	}
	runID := strings.TrimSpace(persistStdout)
	if len(runID) != 32 {
		t.Fatalf("run id stdout=%q stderr=%s", persistStdout, persistStderr)
	}
	if !strings.Contains(persistStderr, "found") {
		t.Fatalf("summary stderr=%q", persistStderr)
	}
}
