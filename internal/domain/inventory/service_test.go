package inventory_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/domain/architecture"
	"github.com/sebihoermann/devdb-go/internal/domain/inventory"
	"github.com/sebihoermann/devdb-go/internal/domain/reminders"
	"github.com/sebihoermann/devdb-go/internal/domain/review"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestScanAndStaleArchNote(t *testing.T) {
	db, _ := testutil.TempDB(t)
	repo := t.TempDir()

	src := filepath.Join(repo, "pkg", "main.go")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatal(err)
	}
	content := []byte("package main\n\nfunc main() {}\n")
	if err := os.WriteFile(src, content, 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := inventory.Scan(db, repo, nil, false, "test")
	if err != nil {
		t.Fatal(err)
	}
	if res.FilesSeen != 1 || res.FilesAdded != 1 {
		t.Fatalf("scan: %+v", res)
	}

	noteID, err := architecture.Add(db, "go-entry", "main entrypoint", []string{"pkg/main.go"}, "high", "test")
	if err != nil {
		t.Fatal(err)
	}

	note, err := architecture.Get(db, noteID)
	if err != nil || note == nil || note.Stale {
		t.Fatalf("fresh note: %+v err=%v", note, err)
	}

	if err := os.WriteFile(src, []byte("package main\n\nfunc main() { println(\"x\") }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := inventory.Scan(db, repo, nil, false, "test"); err != nil {
		t.Fatal(err)
	}

	note, err = architecture.Get(db, noteID)
	if err != nil || note == nil || !note.Stale {
		t.Fatalf("expected stale note after file change: %+v", note)
	}

	status, ok, _, err := architecture.Verify(db, noteID)
	if err != nil || ok || status != "stale" {
		t.Fatalf("verify stale: status=%s ok=%v err=%v", status, ok, err)
	}
}

func TestClassifyFile(t *testing.T) {
	repo := t.TempDir()
	p := filepath.Join(repo, "tests", "test_foo.py")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("def test_x(): pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec, err := inventory.ScanFile(repo, "tests/test_foo.py")
	if err != nil || rec == nil {
		t.Fatal(err)
	}
	if rec.Kind != "test" || rec.Language != "python" {
		t.Fatalf("got %+v", rec)
	}
}

func TestContextTouchingFile(t *testing.T) {
	db, _ := testutil.TempDB(t)
	repo := t.TempDir()
	src := filepath.Join(repo, "lib.go")
	_ = os.WriteFile(src, []byte("package lib\n"), 0o644)
	if _, err := inventory.Scan(db, repo, nil, false, "test"); err != nil {
		t.Fatal(err)
	}
	_, err := architecture.Add(db, "lib-pkg", "library package", []string{"lib.go"}, "medium", "test")
	if err != nil {
		t.Fatal(err)
	}
	payload, err := inventory.Context(db, inventory.ContextOptions{Files: []string{"lib.go"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(payload.ArchitectureNotes) != 1 {
		t.Fatalf("expected 1 note, got %d", len(payload.ArchitectureNotes))
	}
}

func TestLocAndFreshness(t *testing.T) {
	db, _ := testutil.TempDB(t)
	repo := t.TempDir()
	src := filepath.Join(repo, "main.go")
	if err := os.WriteFile(src, []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := inventory.Scan(db, repo, nil, false, "test"); err != nil {
		t.Fatal(err)
	}
	loc, err := inventory.Loc(repo, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if loc.Files < 1 || loc.TotalLines < 2 {
		t.Fatalf("loc=%+v", loc)
	}
	fresh, err := inventory.FreshnessInfo(db)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.FilesIndexed < 1 || fresh.LastScanAt == "" {
		t.Fatalf("freshness=%+v", fresh)
	}
}

func TestContextNoFilesListsActiveNotes(t *testing.T) {
	db, _ := testutil.TempDB(t)
	repo := t.TempDir()
	src := filepath.Join(repo, "a.go")
	if err := os.WriteFile(src, []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := inventory.Scan(db, repo, nil, false, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := architecture.Add(db, "pkg-a", "package a", []string{"a.go"}, "high", "test"); err != nil {
		t.Fatal(err)
	}
	payload, err := inventory.Context(db, inventory.ContextOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(payload.ArchitectureNotes) < 1 {
		t.Fatal("expected active notes without file filter")
	}
}

func TestContextWithFindingsAndReminders(t *testing.T) {
	db, _ := testutil.TempDB(t)
	repo := t.TempDir()
	src := filepath.Join(repo, "svc.go")
	if err := os.WriteFile(src, []byte("package svc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := inventory.Scan(db, repo, nil, false, "test"); err != nil {
		t.Fatal(err)
	}
	runID, err := review.StartRun(db, []string{"."}, "default", "", "test")
	if err != nil {
		t.Fatal(err)
	}
	line := 10
	if _, err := review.AddFinding(db, runID, review.FindingInput{
		FilePath: "svc.go", LineStart: &line, Principle: "kiss", Title: "complex",
		Recommendation: "simplify", Severity: "high", Confidence: "high", Effort: "small",
	}, "test"); err != nil {
		t.Fatal(err)
	}
	remID, err := reminders.Add(db, reminders.AddInput{
		Title: "check svc", FilePath: "svc.go", ModelID: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = remID

	payload, err := inventory.Context(db, inventory.ContextOptions{Files: []string{"svc.go"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(payload.OpenFindings) != 1 || len(payload.FileReminders) != 1 {
		t.Fatalf("findings=%d reminders=%d", len(payload.OpenFindings), len(payload.FileReminders))
	}
	lines := inventory.FormatContextHuman(payload)
	if len(lines) < 5 {
		t.Fatalf("human output too short: %d lines", len(lines))
	}
	if !inventory.ContextStrictExit(payload) {
		t.Fatal("expected strict exit for high finding")
	}
}

func TestDiffSinceGitAware(t *testing.T) {
	db, _ := testutil.TempDB(t)
	repo := t.TempDir()
	testutil.InitGitRepo(t, repo)
	src := filepath.Join(repo, "main.go")
	if err := os.WriteFile(src, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "main.go")
	runGit(t, repo, "commit", "-m", "init main")
	if _, err := inventory.Scan(db, repo, nil, false, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := architecture.Add(db, "main-pkg", "entry", []string{"main.go"}, "high", "test"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rows, err := inventory.DiffSince(db, repo, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || len(rows[0].ArchitectureNotes) != 1 {
		t.Fatalf("diff rows=%+v", rows)
	}
}

func TestScanRemovesDeletedFiles(t *testing.T) {
	db, _ := testutil.TempDB(t)
	repo := t.TempDir()
	src := filepath.Join(repo, "temp.go")
	if err := os.WriteFile(src, []byte("package temp\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := inventory.Scan(db, repo, nil, false, "test")
	if err != nil || res.FilesAdded != 1 {
		t.Fatalf("first scan: %+v err=%v", res, err)
	}
	if err := os.Remove(src); err != nil {
		t.Fatal(err)
	}
	res, err = inventory.Scan(db, repo, nil, false, "test")
	if err != nil || res.FilesRemoved != 1 {
		t.Fatalf("second scan: %+v err=%v", res, err)
	}
}

// TestScanWithNullLanguageAndContentHash is a regression for feedback 2267452b:
// existing repo_files rows with NULL language or NULL content_hash must not
// crash Scan with "converting NULL to string is unsupported".
func TestScanWithNullLanguageAndContentHash(t *testing.T) {
	db, _ := testutil.TempDB(t)
	repo := t.TempDir()

	src := filepath.Join(repo, "pkg", "main.go")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := inventory.Scan(db, repo, nil, false, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`UPDATE repo_files SET language=NULL, content_hash=NULL WHERE path='pkg/main.go'`,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := inventory.Scan(db, repo, nil, false, "test"); err != nil {
		t.Fatalf("scan with NULL language/content_hash must not fail: %v", err)
	}
}
