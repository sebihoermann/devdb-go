package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestFeedbackImportMarkdownHuman(t *testing.T) {
	bin := buildDevdb(t)
	dbPath := filepath.Join(t.TempDir(), "development.db")
	if _, _, code := runDevdb(t, bin, "--db", dbPath, "init"); code != 0 {
		t.Fatalf("init exit %d", code)
	}

	fixture := filepath.Join(moduleRoot(t), "internal", "domain", "feedback", "testdata", "feedback_archive.md")
	stdout, stderr, code := runDevdb(t, bin, "--db", dbPath, "feedback", "import", "markdown", fixture)
	if code != 0 {
		t.Fatalf("import exit %d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "imported 3") {
		t.Fatalf("stdout=%q want imported 3", stdout)
	}
	if !strings.Contains(stderr, "imported 3 row") {
		t.Fatalf("stderr hint missing: %q", stderr)
	}
}

func TestFeedbackImportMarkdownJSON(t *testing.T) {
	bin := buildDevdb(t)
	dbPath := filepath.Join(t.TempDir(), "development.db")
	if _, _, code := runDevdb(t, bin, "--db", dbPath, "init"); code != 0 {
		t.Fatalf("init exit %d", code)
	}

	fixture := filepath.Join(moduleRoot(t), "internal", "domain", "feedback", "testdata", "feedback_archive.md")
	stdout, _, code := runDevdb(t, bin, "--db", dbPath, "--json", "feedback", "import", "markdown", fixture)
	if code != 0 {
		t.Fatalf("import exit %d", code)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &payload); err != nil {
		t.Fatalf("json: %v stdout=%q", err, stdout)
	}
	if int(payload["imported"].(float64)) != 3 {
		t.Fatalf("imported=%v", payload["imported"])
	}
	ids, ok := payload["ids"].([]any)
	if !ok || len(ids) != 3 {
		t.Fatalf("ids=%v", payload["ids"])
	}
}

func TestFeedbackImportCommitsRequiresBranches(t *testing.T) {
	bin := buildDevdb(t)
	dbPath := filepath.Join(t.TempDir(), "development.db")
	if _, _, code := runDevdb(t, bin, "--db", dbPath, "init"); code != 0 {
		t.Fatalf("init exit %d", code)
	}
	_, stderr, code := runDevdb(t, bin, "--db", dbPath, "feedback", "import", "commits")
	if code != 2 {
		t.Fatalf("exit=%d want 2 stderr=%s", code, stderr)
	}
	if !strings.Contains(stderr, "--branches") {
		t.Fatalf("stderr=%q", stderr)
	}
}

func TestFeedbackImportCommitsInGitRepo(t *testing.T) {
	bin := buildDevdb(t)
	dbPath := filepath.Join(t.TempDir(), "development.db")
	if _, _, code := runDevdb(t, bin, "--db", dbPath, "init"); code != 0 {
		t.Fatalf("init exit %d", code)
	}

	root := moduleRoot(t)
	stdout, stderr, code := runDevdb(t, bin, "--repo", root, "--db", dbPath,
		"feedback", "import", "commits", "--branches", "HEAD", "--limit", "5")
	if code != 0 {
		t.Fatalf("import commits exit %d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "inserted") {
		t.Fatalf("stdout=%q", stdout)
	}

	// Idempotent: second run should insert 0.
	stdout2, _, code := runDevdb(t, bin, "--repo", root, "--db", dbPath,
		"feedback", "import", "commits", "--branches", "HEAD", "--limit", "5")
	if code != 0 {
		t.Fatalf("second import exit %d", code)
	}
	if !strings.Contains(stdout2, "inserted 0") {
		t.Fatalf("second stdout=%q want inserted 0", stdout2)
	}
}

func TestLegacyImportFeedbackMDSuggestion(t *testing.T) {
	sug := suggestCommand([]string{"import-feedback-md"})
	if !strings.Contains(sug, "feedback import markdown") {
		t.Fatalf("suggestion=%q", sug)
	}
}

func TestLegacyImportBranchCommitsSuggestion(t *testing.T) {
	sug := suggestCommand([]string{"import-branch-commits"})
	if !strings.Contains(sug, "feedback import commits") {
		t.Fatalf("suggestion=%q", sug)
	}
}
