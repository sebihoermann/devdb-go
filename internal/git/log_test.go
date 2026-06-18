package git

import (
	"testing"

	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestLog(t *testing.T) {
	dir := t.TempDir()
	if _, err := Log(dir, "HEAD", 5); err == nil {
		t.Fatal("expected error outside git repo")
	}
	testutil.InitGitRepo(t, dir)
	commits, err := Log(dir, "HEAD", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 1 {
		t.Fatalf("commits=%d", len(commits))
	}
	if commits[0].SHA == "" || commits[0].Subject == "" {
		t.Fatalf("commit: %+v", commits[0])
	}
	commits, err = Log(dir, "HEAD", 0)
	if err != nil || len(commits) < 1 {
		t.Fatalf("default limit: %d err=%v", len(commits), err)
	}
}
