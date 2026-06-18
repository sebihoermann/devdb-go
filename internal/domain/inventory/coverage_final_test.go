package inventory

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/domain/architecture"
	"github.com/sebihoermann/devdb-go/internal/domain/review"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestScanHelpersWhiteBox(t *testing.T) {
	if !isTransientArtifact("foo.db-wal") || !isTransientArtifact("bar.tmp") || !isTransientArtifact(".#lock") {
		t.Fatal("transient artifacts")
	}
	if isTransientArtifact("main.go") {
		t.Fatal("main.go not transient")
	}
	repo := t.TempDir()
	regular := filepath.Join(repo, "ok.go")
	if err := os.WriteFile(regular, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !isScanable(regular) {
		t.Fatal("regular file scanable")
	}
	if isScanable(filepath.Join(repo, "missing.go")) {
		t.Fatal("missing not scanable")
	}
	if kindFor("README.md", false) != "doc" {
		t.Fatalf("readme kind=%s", kindFor("README.md", false))
	}
	if kindFor("data.bin", true) != "binary" {
		t.Fatal("binary kind")
	}
	if kindFor("tests/test_x.py", false) != "test" {
		t.Fatal("test kind")
	}
	if kindFor("AGENTS.md", false) != "agent_doc" {
		t.Fatal("agent doc")
	}
}

func TestPathInScopeAndNormalize(t *testing.T) {
	if !pathInScope("pkg/a.go", []string{"."}) {
		t.Fatal("dot scope")
	}
	if !pathInScope("pkg/a.go", []string{"pkg"}) {
		t.Fatal("pkg scope")
	}
	if pathInScope("other.go", []string{"pkg"}) {
		t.Fatal("out of scope")
	}
	sc := normalizeScope([]string{"./pkg/"})
	if len(sc) != 1 || sc[0] != "pkg" {
		t.Fatalf("normalize=%v", sc)
	}
}

func TestListFindingsForFilesBranches(t *testing.T) {
	db, _ := testutil.TempDB(t)
	runID, _ := review.StartRun(db, []string{"."}, "default", "", "test")
	_, _ = review.AddFinding(db, runID, review.FindingInput{
		FilePath: "z.go", Principle: "kiss", Title: "open finding",
		Recommendation: "fix", Severity: "low", Confidence: "low", Effort: "trivial",
	}, "test")

	all, err := listFindingsForFiles(db, nil, 5)
	if err != nil || len(all) == 0 {
		t.Fatalf("all open=%d err=%v", len(all), err)
	}
	byFile, err := listFindingsForFiles(db, []string{"z.go", "z.go"}, 5)
	if err != nil || len(byFile) != 1 {
		t.Fatalf("by file=%d err=%v", len(byFile), err)
	}
}

func TestDiffSinceWithGit(t *testing.T) {
	db, _ := testutil.TempDB(t)
	repo := t.TempDir()
	testutil.InitGitRepo(t, repo)
	mainGo := filepath.Join(repo, "main.go")
	if err := os.WriteFile(mainGo, []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", repo, "add", "main.go").CombinedOutput(); err != nil {
		t.Fatalf("add: %v\n%s", err, out)
	}
	if out, err := exec.Command("git", "-C", repo, "commit", "-m", "main").CombinedOutput(); err != nil {
		t.Fatalf("commit: %v\n%s", err, out)
	}
	if _, err := Scan(db, repo, nil, false, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := architecture.Add(db, "diff-topic", "body", []string{"main.go"}, "high", "test"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mainGo, []byte("package main\n\nfunc main() { println(\"v2\") }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rows, err := DiffSince(db, repo, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Fatal("expected diff rows")
	}
}

func TestDiscoverGitAwareAndLoc(t *testing.T) {
	db, _ := testutil.TempDB(t)
	repo := t.TempDir()
	testutil.InitGitRepo(t, repo)
	if err := os.WriteFile(filepath.Join(repo, "tracked.go"), []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", repo, "add", "tracked.go").CombinedOutput(); err != nil {
		t.Fatalf("add: %v\n%s", err, out)
	}
	if out, err := exec.Command("git", "-C", repo, "commit", "-m", "track").CombinedOutput(); err != nil {
		t.Fatalf("commit: %v\n%s", err, out)
	}
	files, err := DiscoverFiles(repo, nil, true)
	if err != nil || len(files) == 0 {
		t.Fatalf("git aware: %v %d", err, len(files))
	}
	if _, err := Scan(db, repo, nil, true, "test"); err != nil {
		t.Fatal(err)
	}
	info, err := Loc(repo, []string{"tracked.go"}, true)
	if err != nil || info.Files == 0 {
		t.Fatalf("loc=%+v err=%v", info, err)
	}
	fresh, err := FreshnessInfo(db)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.FilesIndexed == 0 {
		t.Fatalf("freshness=%+v", fresh)
	}
}

func TestSummarizeBodyTruncation(t *testing.T) {
	long := strings.Join([]string{"line1", "line2", "line3", "line4", "line5"}, "\n")
	if s := summarizeBody(long, 3); !strings.Contains(s, "...") {
		t.Fatalf("summary=%q", s)
	}
	if summarizeBody("short", 5) != "short" {
		t.Fatal("short body unchanged")
	}
}
