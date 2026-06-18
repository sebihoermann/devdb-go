package git

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestGitHelpers(t *testing.T) {
	dir := t.TempDir()
	if HeadSHA(dir) != "" {
		t.Fatal("expected empty sha outside git repo")
	}
	if Branch(dir) != "" {
		t.Fatal("expected empty branch")
	}
	ahead, behind := AheadBehind(dir)
	if ahead != 0 || behind != 0 {
		t.Fatal("expected 0,0 ahead/behind")
	}
	if IsDirty(dir) {
		t.Fatal("expected not dirty")
	}
	_, err := LsFiles(dir, nil)
	if err == nil {
		t.Fatal("expected ls-files error outside repo")
	}
	if ages := BlameLineAges(dir, "missing.go"); len(ages) != 0 {
		t.Fatal("expected empty blame")
	}
	_, err = DiffNameOnly(dir, "HEAD")
	if err == nil {
		t.Fatal("expected diff error outside repo")
	}

	testutil.InitGitRepo(t, dir)
	if HeadSHA(dir) == "" {
		t.Fatal("expected head sha")
	}
	if Branch(dir) == "" {
		t.Fatal("expected branch")
	}
	ahead, behind = AheadBehind(dir)
	if ahead != 0 || behind != 0 {
		t.Fatalf("no upstream: ahead=%d behind=%d", ahead, behind)
	}
	files, err := LsFiles(dir, nil)
	if err != nil || len(files) == 0 {
		t.Fatalf("ls-files: %v %d", err, len(files))
	}
	if err := os.WriteFile(filepath.Join(dir, "dirty.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !IsDirty(dir) {
		t.Fatal("expected dirty")
	}
	ages := BlameLineAges(dir, "README.md")
	if len(ages) == 0 {
		t.Fatal("expected blame ages")
	}
}
