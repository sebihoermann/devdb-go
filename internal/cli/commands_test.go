package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func buildDevdb(t *testing.T) string {
	t.Helper()
	root := moduleRoot(t)
	bin := filepath.Join(t.TempDir(), "devdb")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/devdb")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	return bin
}

func runDevdb(t *testing.T, bin string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return outBuf.String(), errBuf.String(), ee.ExitCode()
		}
		t.Fatal(err)
	}
	return outBuf.String(), errBuf.String(), 0
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return s[:idx]
	}
	return s
}

func TestInitAndStatusGolden(t *testing.T) {
	bin := buildDevdb(t)
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	dbPath := filepath.Join(dir, ".devdb", "development.db")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}

	stdout, _, code := runDevdb(t, bin, "--repo", repo, "--db", dbPath, "init")
	if code != 0 {
		t.Fatalf("init exit %d", code)
	}
	if !strings.Contains(stdout, dbPath) {
		t.Fatalf("init stdout=%q", stdout)
	}

	stdout, _, code = runDevdb(t, bin, "--repo", repo, "--db", dbPath, "status", "--json")
	if code != 0 {
		t.Fatalf("status exit %d stdout=%s", code, stdout)
	}
	if !strings.Contains(stdout, `"overall"`) {
		t.Fatalf("status json=%q", stdout)
	}
}

func TestFeedbackWriteContract(t *testing.T) {
	bin := buildDevdb(t)
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "development.db")

	_, _, code := runDevdb(t, bin, "--db", dbPath, "init")
	if code != 0 {
		t.Fatalf("init failed")
	}

	stdout, _, code := runDevdb(t, bin, "--db", dbPath, "feedback", "add", "--role", "model", "test note")
	if code != 0 {
		t.Fatalf("feedback add exit %d", code)
	}
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 1 || len(lines[0]) != 32 {
		t.Fatalf("want bare 32-char id, got %q", stdout)
	}
}

func TestUnknownCommandSuggestion(t *testing.T) {
	err := unknownCommandError([]string{"work-on"})
	if err.Suggestion == "" {
		t.Fatal("expected suggestion")
	}
}
