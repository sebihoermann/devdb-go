package feedback

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestDetectIntentAllBranches(t *testing.T) {
	cases := map[string]string{
		"":           "other",
		"feat: add x": "feat",
		"fix: bug":    "fix",
		"add widget":  "feat",
		"bug in auth": "fix",
		"error path":  "fix",
		"custom: x":   "custom",
		"docs: readme": "docs",
	}
	for in, want := range cases {
		if got := DetectIntent(in); got != want {
			t.Fatalf("%q => %q want %q", in, got, want)
		}
	}
}

func TestIterMarkdownAndImportText(t *testing.T) {
	text := "## Title one\n- **Role**: model\n- **Category**: ux\n\n## No meta\n\n## Bad role\n- **Role**: alien\n"
	entries := IterMarkdownEntries(text)
	if len(entries) != 3 {
		t.Fatalf("entries=%d", len(entries))
	}
	db, _ := testutil.TempDB(t)
	if _, err := ImportMarkdownText(db, text, "test"); err == nil {
		t.Fatal("expected invalid role error")
	}
	good := "## Good\n- **Role**: user\n- **Story-dir**: /tmp\n- **Context**: ctx\n- **Status**: open\n"
	res, err := ImportMarkdownText(db, good, "test")
	if err != nil || res.Imported != 1 {
		t.Fatalf("import=%+v err=%v", res, err)
	}
}

func TestImportMarkdownFileAndCommits(t *testing.T) {
	dir := t.TempDir()
	md := filepath.Join(dir, "fb.md")
	if err := os.WriteFile(md, []byte("## File\n- **Role**: codebase\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	db, _ := testutil.TempDB(t)
	res, err := ImportMarkdown(db, md, "test")
	if err != nil || res.Imported != 1 {
		t.Fatalf("file import=%+v err=%v", res, err)
	}
	if _, err := ImportMarkdown(db, filepath.Join(dir, "missing.md"), "test"); err == nil {
		t.Fatal("expected read error")
	}
	testutil.InitGitRepo(t, dir)
	commits, err := ImportCommits(db, dir, []string{"HEAD"}, 5, "test")
	if err != nil {
		t.Fatal(err)
	}
	if commits.Inserted < 1 {
		t.Fatalf("commits=%+v", commits)
	}
}
