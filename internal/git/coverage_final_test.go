package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestAheadBehindWithUpstream(t *testing.T) {
	dir := t.TempDir()
	testutil.InitGitRepo(t, dir)
	bare := filepath.Join(t.TempDir(), "origin.git")
	if out, err := exec.Command("git", "init", "--bare", bare).CombinedOutput(); err != nil {
		t.Fatalf("bare: %v\n%s", err, out)
	}
	branch := "master"
	if out, err := exec.Command("git", "-C", dir, "branch", "--show-current").CombinedOutput(); err == nil {
		branch = string(out)
		if len(branch) > 0 && branch[len(branch)-1] == '\n' {
			branch = branch[:len(branch)-1]
		}
	}
	if branch == "" {
		branch = "master"
	}
	for _, args := range [][]string{
		{"git", "-C", dir, "remote", "add", "origin", bare},
		{"git", "-C", dir, "push", "-u", "origin", branch},
	} {
		if out, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	ahead, behind := AheadBehind(dir)
	if ahead != 0 || behind != 0 {
		t.Fatalf("synced: ahead=%d behind=%d", ahead, behind)
	}
	extra := filepath.Join(dir, "extra.go")
	if err := os.WriteFile(extra, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", dir, "add", "extra.go").CombinedOutput(); err != nil {
		t.Fatalf("add: %v\n%s", err, out)
	}
	if out, err := exec.Command("git", "-C", dir, "commit", "-m", "ahead").CombinedOutput(); err != nil {
		t.Fatalf("commit: %v\n%s", err, out)
	}
	ahead, behind = AheadBehind(dir)
	if ahead < 1 {
		t.Fatalf("expected ahead>=1 got ahead=%d behind=%d", ahead, behind)
	}
}

func TestDiffNameOnlyAndLsFilesPaths(t *testing.T) {
	dir := t.TempDir()
	testutil.InitGitRepo(t, dir)
	mainGo := filepath.Join(dir, "main.go")
	if err := os.WriteFile(mainGo, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", dir, "add", "main.go").CombinedOutput(); err != nil {
		t.Fatalf("add: %v\n%s", err, out)
	}
	if out, err := exec.Command("git", "-C", dir, "commit", "-m", "main").CombinedOutput(); err != nil {
		t.Fatalf("commit: %v\n%s", err, out)
	}
	if err := os.WriteFile(mainGo, []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	changed, err := DiffNameOnly(dir, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) == 0 || changed[0] != "main.go" {
		t.Fatalf("diff=%v", changed)
	}
	files, err := LsFiles(dir, []string{"main.go"})
	if err != nil || len(files) != 1 {
		t.Fatalf("ls-files scoped: %v err=%v", files, err)
	}
}

func TestBlameInvalidAuthorTime(t *testing.T) {
	dir := t.TempDir()
	testutil.InitGitRepo(t, dir)
	readme := filepath.Join(dir, "README.md")
	ages := BlameLineAges(dir, readme)
	if len(ages) == 0 {
		t.Fatal("expected blame ages from init commit")
	}
}
