package grasscutter

import (
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestPathInScope(t *testing.T) {
	scope := []string{"pkg", "other/deep"}
	cases := []struct {
		path string
		want bool
	}{
		{"pkg/main.py", true},
		{"pkg/sub/x.py", true},
		{"other/deep/file.py", true},
		{"other/shallow.py", false},
		{"lib.py", false},
	}
	for _, tc := range cases {
		if got := pathInScope(tc.path, scope); got != tc.want {
			t.Fatalf("pathInScope(%q)=%v want %v", tc.path, got, tc.want)
		}
	}
	if !pathInScope("any.py", []string{"."}) {
		t.Fatal("dot scope")
	}
	if !pathInScope("any.py", []string{""}) {
		t.Fatal("empty scope item")
	}
}

func TestPythonFilesScopeFilter(t *testing.T) {
	db, _ := testutil.TempDB(t)
	now := "2020-01-01T00:00:00Z"
	for _, p := range []string{"pkg/a.py", "pkg/b.py", "lib/c.py"} {
		if _, err := db.Exec(
			`INSERT INTO repo_files(path, kind, last_seen_at) VALUES (?, 'code', ?)`, p, now,
		); err != nil {
			t.Fatal(err)
		}
	}
	all, err := pythonFiles(db, nil)
	if err != nil || len(all) != 3 {
		t.Fatalf("all files: %d err=%v", len(all), err)
	}
	filtered, err := pythonFiles(db, []string{"pkg"})
	if err != nil || len(filtered) != 2 {
		t.Fatalf("scoped files: %v err=%v", filtered, err)
	}
}

func TestBlameAgeDaysUsesCache(t *testing.T) {
	repo := t.TempDir()
	testutil.InitGitRepo(t, repo)
	cache := map[string]map[int]int{}
	age := blameAgeDays(repo, "README.md", 1, cache)
	if age == nil || *age < 0 {
		t.Fatalf("blame age: %v", age)
	}
	if len(cache["README.md"]) == 0 {
		t.Fatal("expected cache population")
	}
	missing := blameAgeDays(repo, "README.md", 99999, cache)
	if missing != nil {
		t.Fatalf("missing line should be nil, got %d", *missing)
	}
}

func TestDiscoverStalenessOldTODO(t *testing.T) {
	repo := t.TempDir()
	pyPath := filepath.Join(repo, "stale.py")
	if err := os.WriteFile(pyPath, []byte("# TODO ancient fix\npass\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test User"},
		{"git", "add", "."},
	} {
		cmd := exec.Command(args[0], append(args[1:], repo)...)
		if args[1] == "config" {
			cmd = exec.Command("git", "-C", repo, "config", args[2], args[3])
		} else if args[1] == "add" {
			cmd = exec.Command("git", "-C", repo, "add", ".")
		} else if args[1] == "init" {
			cmd = exec.Command("git", "init", repo)
		}
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
	}
	commit := exec.Command("git", "-C", repo, "-c", "user.email=test@test.com", "-c", "user.name=Test",
		"commit", "--date=2019-06-01T12:00:00", "-m", "old todo")
	if out, err := commit.CombinedOutput(); err != nil {
		t.Fatalf("commit: %s", out)
	}

	db, _ := testutil.TempDB(t)
	if _, err := db.Exec(
		`INSERT INTO repo_files(path, kind, lines, last_seen_at) VALUES ('stale.py', 'code', 2, '2020-01-01T00:00:00Z')`,
	); err != nil {
		t.Fatal(err)
	}
	_, counts, err := Discover(repo, db, []string{"."}, []string{"staleness"})
	if err != nil {
		t.Fatal(err)
	}
	if counts["staleness"] < 1 {
		t.Fatalf("expected staleness candidates, counts=%v", counts)
	}
}

func TestDiscoverReadPythonError(t *testing.T) {
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, "broken.py"), 0o755); err != nil {
		t.Fatal(err)
	}
	db := mustDBWithPyFile(t, "broken.py")
	_, _, err := Discover(repo, db, []string{"."}, []string{"dead"})
	if err == nil {
		t.Fatal("expected read error for directory posing as .py")
	}
}

func TestCapCandidatesAndFormatSummary(t *testing.T) {
	cands := []Candidate{
		{FilePath: "a.py", Principle: "dead", Severity: "high"},
		{FilePath: "a.py", Principle: "dead", Severity: "low"},
		{FilePath: "b.py", Principle: "sprawl", Severity: "critical"},
	}
	kept := capCandidates(cands, 1)
	if len(kept) != 2 {
		t.Fatalf("cap kept=%d want 2", len(kept))
	}
	summary := formatSummary(cands, map[string]int{"dead": 2, "sprawl": 1})
	if summary == "" || summary[:5] != "found" {
		t.Fatalf("summary: %q", summary)
	}
}

func mustDBWithPyFile(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, _ := testutil.TempDB(t)
	if _, err := db.Exec(
		`INSERT INTO repo_files(path, kind, lines, last_seen_at) VALUES (?, 'code', 1, '2020-01-01T00:00:00Z')`, path,
	); err != nil {
		t.Fatal(err)
	}
	return db
}
