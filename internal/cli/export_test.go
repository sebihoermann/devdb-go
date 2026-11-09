package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// exportRepo creates a repository with an initialized ledger and returns the
// binary, the repository root, and the database path inside it.
func exportRepo(t *testing.T) (bin, repo, dbPath string) {
	t.Helper()
	bin = buildDevdb(t)
	dir := t.TempDir()
	repo = filepath.Join(dir, "repo")
	dbPath = filepath.Join(repo, ".devdb", "development.db")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	_, _, code := runDevdb(t, bin, "--repo", repo, "--db", dbPath, "init")
	if code != 0 {
		t.Fatalf("init exit %d", code)
	}
	return bin, repo, dbPath
}

func TestLedgerExportPrintsProseToStdout(t *testing.T) {
	bin, repo, dbPath := exportRepo(t)
	stdout, stderr, code := runDevdb(t, bin, "--repo", repo, "--db", dbPath, "ledger", "export")
	if code != 0 {
		t.Fatalf("export exit %d stderr=%s", code, stderr)
	}
	if !strings.HasPrefix(stdout, "# Ledger report") {
		t.Fatalf("stdout is not the report:\n%s", stdout)
	}
	for _, want := range []string{"## Summary", "## Delivery", "## Quality", "## Feedback"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("report missing %q", want)
		}
	}
	if stderr != "" {
		t.Fatalf("stderr should be empty on the stdout path: %q", stderr)
	}
}

func TestLedgerExportWritesFileAndPrintsThePath(t *testing.T) {
	bin, repo, dbPath := exportRepo(t)
	out := filepath.Join(repo, "docs", "ledger.md")
	stdout, stderr, code := runDevdb(t, bin, "--repo", repo, "--db", dbPath,
		"ledger", "export", "--output", out)
	if code != 0 {
		t.Fatalf("export exit %d stderr=%s", code, stderr)
	}
	if strings.TrimSpace(stdout) != out {
		t.Fatalf("stdout should be the written path, got %q", stdout)
	}
	if !strings.Contains(stderr, "wrote") {
		t.Fatalf("stderr should say it wrote the file, got %q", stderr)
	}
	body, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(body), "# Ledger report") {
		t.Fatalf("written file is not the report:\n%s", body)
	}
}

func TestLedgerExportRejectsUnknownView(t *testing.T) {
	bin, repo, dbPath := exportRepo(t)
	stdout, stderr, code := runDevdb(t, bin, "--repo", repo, "--db", dbPath,
		"ledger", "export", "--view", "bogus")
	if code != ExitUsage {
		t.Fatalf("exit %d, want %d (stderr=%q)", code, ExitUsage, stderr)
	}
	if stdout != "" {
		t.Fatalf("a rejected view must not print a report: %q", stdout)
	}
	if !strings.Contains(stderr, "known: summary") {
		t.Fatalf("error does not name the known views: %q", stderr)
	}
}

func TestLedgerExportJSONCarriesMetadata(t *testing.T) {
	bin, repo, dbPath := exportRepo(t)
	out := filepath.Join(repo, "ledger.md")
	stdout, _, code := runDevdb(t, bin, "--repo", repo, "--db", dbPath, "--json",
		"ledger", "export", "--output", out, "--view", "summary")
	if code != 0 {
		t.Fatalf("export exit %d", code)
	}
	var meta struct {
		Path  string   `json:"path"`
		Bytes int      `json:"bytes"`
		Views []string `json:"views"`
	}
	if err := json.Unmarshal([]byte(stdout), &meta); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout)
	}
	if meta.Path != out {
		t.Fatalf("path=%q want %q", meta.Path, out)
	}
	if meta.Bytes <= 0 {
		t.Fatalf("bytes=%d", meta.Bytes)
	}
	if len(meta.Views) != 1 || meta.Views[0] != "summary" {
		t.Fatalf("views=%v", meta.Views)
	}
}

func TestLedgerExportViewNarrowsTheDocument(t *testing.T) {
	bin, repo, dbPath := exportRepo(t)
	stdout, _, code := runDevdb(t, bin, "--repo", repo, "--db", dbPath,
		"ledger", "export", "--view", "summary")
	if code != 0 {
		t.Fatalf("export exit %d", code)
	}
	if !strings.Contains(stdout, "## Summary") {
		t.Fatalf("selected view missing:\n%s", stdout)
	}
	if strings.Contains(stdout, "## Delivery") {
		t.Fatalf("unselected view rendered:\n%s", stdout)
	}
}

func TestLedgerExportIsStableAcrossRuns(t *testing.T) {
	bin, repo, dbPath := exportRepo(t)
	first, _, code := runDevdb(t, bin, "--repo", repo, "--db", dbPath, "ledger", "export")
	if code != 0 {
		t.Fatalf("first export exit %d", code)
	}
	second, _, code := runDevdb(t, bin, "--repo", repo, "--db", dbPath, "ledger", "export")
	if code != 0 {
		t.Fatalf("second export exit %d", code)
	}
	if first != second {
		t.Fatal("two exports of an unchanged ledger differ")
	}
}

// TestLedgerExportGolden pins the rendered report.
//
// The report is prose, so the golden is what keeps a future edit from
// quietly rewriting the document: a changed sentence is a changed golden and
// has to be read on purpose.
func TestLedgerExportGolden(t *testing.T) {
	seed := seedGoldenDB(t)
	base := []string{"--repo", seed.repo, "--db", seed.dbPath}
	stdout, stderr, code := runCLI(t, append(base, "ledger", "export")...)
	if code != 0 {
		t.Fatalf("exit %d stderr=%s", code, stderr)
	}
	assertGolden(t, "ledger_export.golden", normalizeGolden(stdout))
}
