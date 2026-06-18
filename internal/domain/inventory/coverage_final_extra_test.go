package inventory

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/domain/architecture"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestContextStrictExitHighFinding(t *testing.T) {
	db, _ := testutil.TempDB(t)
	repo := t.TempDir()
	p := filepath.Join(repo, "h.go")
	if err := os.WriteFile(p, []byte("package h\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Scan(db, repo, nil, false, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO review_runs(id, scope_paths, tier, started_at, model_id) VALUES ('r1','[]','default',datetime('now'),'test')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO review_findings(id, run_id, file_path, principle, title, recommendation, severity, confidence, effort, status, created_at, model_id)
		VALUES ('f1','r1','h.go','kiss','critical','fix','high','high','small','open',datetime('now'),'test')`); err != nil {
		t.Fatal(err)
	}
	payload, err := Context(db, ContextOptions{Files: []string{"h.go"}, Task: "edit"})
	if err != nil {
		t.Fatal(err)
	}
	if !ContextStrictExit(payload) {
		t.Fatal("expected strict exit for high finding")
	}
}

func TestPathMatchesGitignoreDirectory(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte("build/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !pathMatchesGitignore(repo, "build/out") {
		t.Fatal("build/ prefix should match")
	}
}

func TestFormatContextHumanEmpty(t *testing.T) {
	lines := FormatContextHuman(ContextPayload{})
	if len(lines) == 0 {
		t.Fatal("expected header lines")
	}
}

func TestListFindingsNoTable(t *testing.T) {
	db, _ := testutil.TempDB(t)
	if _, err := db.Exec(`DROP TABLE review_findings`); err != nil {
		t.Fatal(err)
	}
	out, err := listFindingsForFiles(db, []string{"x.go"}, 5)
	if err != nil || out != nil {
		t.Fatalf("out=%v err=%v", out, err)
	}
}

func TestDiffSinceWithArchNotes(t *testing.T) {
	db, _ := testutil.TempDB(t)
	repo := t.TempDir()
	testutil.InitGitRepo(t, repo)
	main := filepath.Join(repo, "main.go")
	if err := os.WriteFile(main, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "main.go")
	runGit(t, repo, "commit", "-m", "init")
	if _, err := Scan(db, repo, nil, false, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := architecture.Add(db, "entry", "body", []string{"main.go"}, "high", "test"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(main, []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rows, err := DiffSince(db, repo, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 || len(rows[0].ArchitectureNotes) == 0 {
		t.Fatalf("rows=%+v", rows)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := append([]string{"git", "-C", dir}, args...)
	if out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
