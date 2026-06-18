package feedback_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/domain/feedback"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestDetectIntent(t *testing.T) {
	cases := map[string]string{
		"feat: add widget":     "feat",
		"fix(api): handle err": "fix",
		"add new endpoint":     "feat",
		"broken login flow":    "fix",
		"random note":          "other",
		"":                     "other",
	}
	for text, want := range cases {
		if got := feedback.DetectIntent(text); got != want {
			t.Fatalf("DetectIntent(%q)=%q want %q", text, got, want)
		}
	}
}

func TestIterMarkdownEntries(t *testing.T) {
	text := "## My Note\n- **Role**: model\n- **Severity**: high\n\n## Another\n- **Category**: bug\n"
	entries := feedback.IterMarkdownEntries(text)
	if len(entries) != 2 {
		t.Fatalf("entries=%d want 2", len(entries))
	}
	if entries[0].Title != "My Note" || !entries[0].HasMeta {
		t.Fatalf("first entry: %+v", entries[0])
	}
	if entries[0].Meta["role"] != "model" {
		t.Fatalf("role=%q", entries[0].Meta["role"])
	}
	if entries[1].Title != "Another" || entries[1].Meta["category"] != "bug" {
		t.Fatalf("second entry: %+v", entries[1])
	}
}

func TestIterMarkdownEntriesSkipsHeadingWithoutMeta(t *testing.T) {
	text := "## Header only\n\n## Good\n- **Role**: user\n"
	entries := feedback.IterMarkdownEntries(text)
	if len(entries) != 2 {
		t.Fatalf("entries=%d want 2", len(entries))
	}
	if entries[0].HasMeta {
		t.Fatal("header-only entry should not have meta")
	}
	if !entries[1].HasMeta {
		t.Fatal("good entry should have meta")
	}
}

func TestImportMarkdownFixture(t *testing.T) {
	db, _ := testutil.TempDB(t)
	path := filepath.Join("testdata", "feedback_archive.md")
	result, err := feedback.ImportMarkdown(db, path, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Imported != 3 {
		t.Fatalf("imported=%d want 3 (skips header-only)", result.Imported)
	}
	if len(result.IDs) != 3 {
		t.Fatalf("ids=%d want 3", len(result.IDs))
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM feedback`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("feedback rows=%d", count)
	}

	var role, severity, status, fix string
	err = db.QueryRow(`SELECT role, severity, status, COALESCE(proposed_fix,'') FROM feedback WHERE note='Good note'`).
		Scan(&role, &severity, &status, &fix)
	if err != nil {
		t.Fatal(err)
	}
	if role != "model" || severity != "high" || status != "open" {
		t.Fatalf("good note: role=%s severity=%s status=%s", role, severity, status)
	}

	err = db.QueryRow(`SELECT status, COALESCE(proposed_fix,'') FROM feedback WHERE note='Closed item'`).
		Scan(&status, &fix)
	if err != nil {
		t.Fatal(err)
	}
	if status != "closed" || fix != "shipped in abc123" {
		t.Fatalf("closed item: status=%s fix=%q", status, fix)
	}

	var defaultRole string
	if err := db.QueryRow(`SELECT role FROM feedback WHERE note='Codebase default'`).Scan(&defaultRole); err != nil {
		t.Fatal(err)
	}
	if defaultRole != "codebase" {
		t.Fatalf("default role=%s", defaultRole)
	}
}

func TestImportMarkdownMissingFile(t *testing.T) {
	db, _ := testutil.TempDB(t)
	_, err := feedback.ImportMarkdown(db, "/no/such/file.md", "test")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestImportMarkdownTextInvalidRole(t *testing.T) {
	db, _ := testutil.TempDB(t)
	text := "## Bad\n- **Role**: admin\n"
	_, err := feedback.ImportMarkdownText(db, text, "test")
	if err == nil {
		t.Fatal("expected invalid role error")
	}
}

func TestImportMarkdownFromRepoFixture(t *testing.T) {
	// Ensure fixture path resolves when tests run from package dir.
	_, err := os.Stat(filepath.Join("testdata", "feedback_archive.md"))
	if err != nil {
		t.Fatalf("fixture missing: %v", err)
	}
}

func TestImportCommits(t *testing.T) {
	db, _ := testutil.TempDB(t)
	repo := t.TempDir()
	testutil.InitGitRepo(t, repo)

	result, err := feedback.ImportCommits(db, repo, []string{"HEAD"}, 10, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Inserted < 1 {
		t.Fatalf("inserted=%d want >=1", result.Inserted)
	}

	// idempotent second import
	result2, err := feedback.ImportCommits(db, repo, []string{"HEAD"}, 10, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result2.Inserted != 0 {
		t.Fatalf("second insert=%d want 0", result2.Inserted)
	}
}

func TestImportCommitsBadBranch(t *testing.T) {
	db, _ := testutil.TempDB(t)
	repo := t.TempDir()
	testutil.InitGitRepo(t, repo)
	result, err := feedback.ImportCommits(db, repo, []string{"no-such-branch"}, 5, "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("warnings=%v", result.Warnings)
	}
}
