package hub

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/domain/feedback"
	"github.com/sebihoermann/devdb-go/internal/domain/review"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestAcrossBuiltinQueriesWithData(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	dbPath := filepath.Join(repo, ".devdb", "development.db")
	registry := filepath.Join(dir, "registry")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	srcDB, srcPath := testutil.TempDB(t)
	runID, _ := review.StartRun(srcDB, []string{"."}, "default", "", "test")
	_, _ = review.AddFinding(srcDB, runID, review.FindingInput{
		FilePath: "a.go", Principle: "dry", Title: "dup", Recommendation: "extract",
		Severity: "high", Confidence: "high", Effort: "small",
	}, "test")
	_, _ = review.FinishRun(srcDB, runID, "done")
	_, _ = feedback.Add(srcDB, feedback.AddInput{
		Role: "model", Category: "bug", Severity: "high", Note: "open debt issue", ModelID: "test",
	})
	_ = srcDB.Close()
	data, _ := os.ReadFile(srcPath)
	_ = os.MkdirAll(filepath.Dir(dbPath), 0o755)
	if err := os.WriteFile(dbPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(registry, []byte(repo+" across\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Register(repo, "across", registry, filepath.Join(dir, "meta.db")); err != nil {
		t.Fatal(err)
	}

	for _, query := range []string{"open-debt", "code-hygiene-cross", "similar-feedback"} {
		rows, err := Across(AcrossOptions{Query: query, Keyword: "debt", Registry: registry})
		if err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		if len(rows) == 0 {
			t.Fatalf("%s: expected rows", query)
		}
	}
	_, err := Across(AcrossOptions{Query: "nope", Registry: registry})
	if err == nil {
		t.Fatal("expected unknown query error")
	}
}

func TestReadRegistryCommentsAndAlias(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "myproject")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(repo, ".devdb", "development.db")
	_ = os.MkdirAll(filepath.Dir(dbPath), 0o755)
	srcDB, srcPath := testutil.TempDB(t)
	_ = srcDB.Close()
	data, _ := os.ReadFile(srcPath)
	if err := os.WriteFile(dbPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	reg := filepath.Join(dir, "reg")
	content := "# federation registry\n\n" + repo + " custom-alias\n"
	if err := os.WriteFile(reg, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	projects, err := ReadRegistry(reg)
	if err != nil || len(projects) != 1 {
		t.Fatalf("projects=%v err=%v", projects, err)
	}
	if projects[0].Alias != "custom-alias" || !projects[0].Exists {
		t.Fatalf("entry=%+v", projects[0])
	}
}

func TestSyncAllStrictWithMissingProject(t *testing.T) {
	dir := t.TempDir()
	registry := filepath.Join(dir, "reg")
	metaDB := filepath.Join(dir, "meta.db")
	missing := filepath.Join(dir, "missing-repo")
	if err := os.WriteFile(registry, []byte(missing+" ghost\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := SyncAll(registry, metaDB, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "failed" || res.ProjectsFailed < 1 {
		t.Fatalf("strict sync=%+v", res)
	}
}

func TestBuiltinQueryNamesAndPaths(t *testing.T) {
	names := BuiltinQueryNames()
	if len(names) < 3 {
		t.Fatalf("names=%v", names)
	}
	t.Setenv("DEVDB_METADATA_DB", filepath.Join(t.TempDir(), "custom-meta.db"))
	if got := ResolveMetadataDB(""); got == "" {
		t.Fatal("empty metadata db")
	}
	if got := ResolveMetadataDB("/explicit.db"); got != "/explicit.db" {
		t.Fatalf("explicit=%q", got)
	}
	if got := ResolveRegistry("/explicit-reg"); got != "/explicit-reg" {
		t.Fatalf("registry=%q", got)
	}
	if alias := defaultAlias("my-repo"); alias != "my-repo" {
		t.Fatalf("alias=%q", alias)
	}
	if alias := defaultAlias("/path/with spaces"); alias != "with-spaces" {
		t.Fatalf("spaced alias=%q", alias)
	}
}

func TestOpenHubClosedDBErrors(t *testing.T) {
	db := testutil.ClosedDB(t)
	ok, msg := SyncOne(db, RegisteredProject{Alias: "x", Root: t.TempDir(), DBPath: "nope", Exists: true})
	if ok || msg == "" {
		t.Fatalf("sync closed hub: ok=%v msg=%q", ok, msg)
	}
}

func TestEncodeSnapshotRoundTrip(t *testing.T) {
	snap := Snapshot{
		SyncStatus: "active", OpenFeedback: 2, InFlightTitle: "work",
		GitBranch: "main", GitDirty: true,
	}
	raw, err := encodeSnapshot(snap)
	if err != nil || raw == "" {
		t.Fatalf("encode: %q err=%v", raw, err)
	}
	dec, err := decodeSnapshot(raw)
	if err != nil || dec.InFlightTitle != "work" || !dec.GitDirty {
		t.Fatalf("decode=%+v err=%v", dec, err)
	}
}

func TestExpandHomePath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	got := expandHome("~/devdb-test")
	if !strings.HasPrefix(got, home) {
		t.Fatalf("expand=%q", got)
	}
	if expandHome("/abs/path") != "/abs/path" {
		t.Fatal("absolute path changed")
	}
}

func TestWriteRegistryRoundTrip(t *testing.T) {
	dir := t.TempDir()
	reg := filepath.Join(dir, "nested", "registry")
	projects := []RegisteredProject{
		{Alias: "b", Root: filepath.Join(dir, "b")},
		{Alias: "a", Root: filepath.Join(dir, "a")},
	}
	if err := WriteRegistry(reg, projects); err != nil {
		t.Fatal(err)
	}
	read, err := ReadRegistry(reg)
	if err != nil || len(read) != 2 {
		t.Fatalf("read=%v err=%v", read, err)
	}
	if read[0].Alias != "a" || read[1].Alias != "b" {
		t.Fatalf("sorted=%v", read)
	}
}
