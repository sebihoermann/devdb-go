package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReviewVerifySession(t *testing.T) {
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
	run("inventory", "scan")
	runID := run("review", "start", "--paths", ".")
	findingID := run("review", "finding",
		"--run", runID,
		"--file", "main.go",
		"--principle", "kiss",
		"--title", "entry point",
		"--recommendation", "document package",
		"--severity", "low",
	)
	if len(findingID) != 32 {
		t.Fatalf("finding id: %q", findingID)
	}
	run("review", "finish", runID, "--summary", "smoke review")

	listOut, _, code := runDevdb(t, bin, "--repo", repo, "--db", dbPath, "review", "list", "--run", runID)
	if code != 0 || !strings.Contains(listOut, "kiss") {
		t.Fatalf("list: code=%d out=%q", code, listOut)
	}

	vRun := run("verify", "record", "go test ./...", "--scope", ".", "--status", "passed", "--exit-code", "0", "--finished", "--git-sha", "abc123")
	queryOut, _, code := runDevdb(t, bin, "--repo", repo, "--db", dbPath,
		"verify", "query", "--command", "go test ./...", "--scope", ".")
	if code != 0 || !strings.Contains(queryOut, "skip rerun") {
		t.Fatalf("query: code=%d out=%q", code, queryOut)
	}

	showOut, _, code := runDevdb(t, bin, "--repo", repo, "--db", dbPath, "verify", "show", vRun)
	if code != 0 || !strings.Contains(showOut, "fresh_pass") {
		t.Fatalf("show: code=%d out=%q", code, showOut)
	}

	principlesOut, _, code := runDevdb(t, bin, "--repo", repo, "--db", dbPath, "review", "principles")
	if code != 0 || !strings.Contains(principlesOut, "kiss") {
		t.Fatalf("principles: %q", principlesOut)
	}
}
