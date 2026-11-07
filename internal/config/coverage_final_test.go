package config

import (
	"os"
	"path/filepath"
	"testing"
)

// chainHasMarkers reports whether dir or any of its ancestors contains a
// .devdb/ or .git marker — the two roots resolveRepoRoot stops at.
func chainHasMarkers(dir string) bool {
	for {
		for _, m := range []string{DevDBDirName, ".git"} {
			if _, err := os.Stat(filepath.Join(dir, m)); err == nil {
				return true
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return false
		}
		dir = parent
	}
}

// markerFreeTempDir returns a fresh temp dir whose ENTIRE ancestor chain is
// free of .devdb/.git markers, so the cwd-fallback branch of resolveRepoRoot
// is actually exercised. t.TempDir() honors $TMPDIR, and hosts that run devdb
// on themselves (e.g. hermes: TMPDIR under a home dir that holds the hub's
// .devdb/) make the plain temp dir's chain marked — the walk-up then stops at
// that ancestor instead of falling back to cwd, which was a test-environment
// failure, not a product regression (dogfood finding 1c77b5dc).
func markerFreeTempDir(t *testing.T) string {
	t.Helper()
	candidates := []string{os.TempDir()}
	if alt := filepath.Join(string(os.PathSeparator), "tmp"); alt != os.TempDir() {
		candidates = append(candidates, alt)
	}
	for _, base := range candidates {
		if base == "" {
			continue
		}
		if _, err := os.Stat(base); err != nil {
			continue
		}
		if chainHasMarkers(base) {
			continue
		}
		dir, err := os.MkdirTemp(base, "resolve-fallback")
		if err != nil {
			continue
		}
		t.Cleanup(func() { _ = os.RemoveAll(dir) })
		return dir
	}
	t.Skipf("no marker-free temp base available (TMPDIR=%q chain contains .devdb/.git); cannot exercise the cwd-fallback branch", os.TempDir())
	return ""
}

func TestResolveFallbackCwd(t *testing.T) {
	dir := markerFreeTempDir(t)
	sub := filepath.Join(dir, "deep", "nested")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(sub); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	proj, err := Resolve("", "")
	if err != nil {
		t.Fatal(err)
	}
	absSub, _ := filepath.Abs(sub)
	if proj.RepoRoot != absSub {
		t.Fatalf("repo=%q want %q", proj.RepoRoot, absSub)
	}
}

func TestResolveRepoFlagAbs(t *testing.T) {
	dir := t.TempDir()
	rel := "."
	proj, err := Resolve(rel, "")
	if err != nil {
		t.Fatal(err)
	}
	abs, _ := filepath.Abs(rel)
	if proj.RepoRoot != abs {
		t.Fatalf("abs repo=%q", proj.RepoRoot)
	}
	customDB := filepath.Join(dir, "x.db")
	proj, err = Resolve(dir, customDB)
	if err != nil || proj.DBPath != customDB {
		t.Fatalf("custom db=%q err=%v", proj.DBPath, err)
	}
}
