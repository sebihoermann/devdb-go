package hub_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/domain/feedback"
	"github.com/sebihoermann/devdb-go/internal/domain/hub"
	"github.com/sebihoermann/devdb-go/internal/domain/review"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestBuiltinQueryNames(t *testing.T) {
	names := hub.BuiltinQueryNames()
	if len(names) != 3 {
		t.Fatalf("names=%v", names)
	}
}

func TestAcrossUnknownQuery(t *testing.T) {
	_, err := hub.Across(hub.AcrossOptions{Query: "nope", Registry: t.TempDir() + "/reg"})
	if err == nil {
		t.Fatal("expected unknown query error")
	}
}

func TestAcrossHygieneCross(t *testing.T) {
	dir := t.TempDir()
	registry := filepath.Join(dir, "registry")
	metaDB := filepath.Join(dir, "metadata.db")
	root := setupAcrossProject(t, dir, "hygiene")

	db, path := testutil.TempDB(t)
	runID, _ := review.StartRun(db, []string{"."}, "default", "", "test")
	_, _ = review.AddFinding(db, runID, review.FindingInput{
		FilePath: "a.go", Principle: "dry", Title: "dup", Recommendation: "extract",
		Severity: "high", Confidence: "high", Effort: "small",
	}, "test")
	db.Close()
	moveDB(t, path, root)

	if _, err := hub.Register(root, "hygiene", registry, metaDB); err != nil {
		t.Fatal(err)
	}
	rows, err := hub.Across(hub.AcrossOptions{Query: "code-hygiene-cross", Registry: registry})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["project"] != "hygiene" {
		t.Fatalf("rows=%v", rows)
	}
}

func TestAcrossSimilarFeedback(t *testing.T) {
	dir := t.TempDir()
	registry := filepath.Join(dir, "registry")
	metaDB := filepath.Join(dir, "metadata.db")
	root := setupAcrossProject(t, dir, "feedback")

	db, path := testutil.TempDB(t)
	_, _ = feedback.Add(db, feedback.AddInput{
		Role: "user", Category: "ux", Note: "button color mismatch", ModelID: "test",
	})
	db.Close()
	moveDB(t, path, root)

	if _, err := hub.Register(root, "feedback", registry, metaDB); err != nil {
		t.Fatal(err)
	}
	rows, err := hub.Across(hub.AcrossOptions{
		Query: "similar-feedback", Keyword: "button", Category: "ux", Registry: registry,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows=%v", rows)
	}
}

func setupAcrossProject(t *testing.T, dir, name string) string {
	t.Helper()
	root := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Join(root, ".devdb"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func moveDB(t *testing.T, src, root string) {
	t.Helper()
	devdbDir := filepath.Join(root, ".devdb")
	if err := os.Rename(src, filepath.Join(devdbDir, "development.db")); err != nil {
		t.Fatal(err)
	}
}

func TestAcrossSeverityRanksAllLevels(t *testing.T) {
	dir := t.TempDir()
	registry := filepath.Join(dir, "registry")
	metaDB := filepath.Join(dir, "metadata.db")
	root := setupAcrossProject(t, dir, "ranks")

	db, path := testutil.TempDB(t)
	_, err := db.Exec(`INSERT INTO review_runs(id, scope_paths, tier, started_at, model_id) VALUES ('r1','.','default',datetime('now'),'test')`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO review_findings(
		id, run_id, principle, title, recommendation, severity, confidence, effort, status, created_at, model_id
	) VALUES
	 ('f1','r1','dry','low issue','fix','low','high','small','open',datetime('now','-4 hour'),'test'),
	 ('f2','r1','kiss','med issue','fix','med','high','small','open',datetime('now','-3 hour'),'test'),
	 ('f3','r1','yagni','medium issue','fix','medium','high','small','open',datetime('now','-2 hour'),'test'),
	 ('f4','r1','dry','unknown issue','fix','info','high','small','open',datetime('now','-1 hour'),'test')`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	moveDB(t, path, root)

	if _, err := hub.Register(root, "ranks", registry, metaDB); err != nil {
		t.Fatal(err)
	}
	rows, err := hub.Across(hub.AcrossOptions{Query: "code-hygiene-cross", Registry: registry})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 4 {
		t.Fatalf("rows=%d", len(rows))
	}
	// medium (rank 3) should sort before low (2) and info/default (1)
	if rows[0]["severity"] != "medium" && rows[0]["severity"] != "med" {
		t.Fatalf("expected med/medium first, got %v", rows[0]["severity"])
	}
}

func TestAcrossSkipsMissingProjects(t *testing.T) {
	dir := t.TempDir()
	registry := filepath.Join(dir, "registry")
	if err := os.WriteFile(registry, []byte(filepath.Join(dir, "ghost")+" ghost\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rows, err := hub.Across(hub.AcrossOptions{Query: "open-debt", Registry: registry})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected no rows for missing db, got %v", rows)
	}
}

func TestAcrossSkipsCorruptProjectDB(t *testing.T) {
	dir := t.TempDir()
	registry := filepath.Join(dir, "registry")
	root := setupAcrossProject(t, dir, "corrupt")
	if err := os.WriteFile(filepath.Join(root, ".devdb", "development.db"), []byte("bad"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(registry, []byte(root+" corrupt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rows, err := hub.Across(hub.AcrossOptions{Query: "open-debt", Registry: registry})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("rows=%v", rows)
	}
}

func TestAcrossSimilarFeedbackKeywordOnly(t *testing.T) {
	dir := t.TempDir()
	registry := filepath.Join(dir, "registry")
	root := setupAcrossProject(t, dir, "kw")

	db, path := testutil.TempDB(t)
	_, _ = feedback.Add(db, feedback.AddInput{
		Role: "user", Note: "unique-keyword-token here", ModelID: "test",
	})
	db.Close()
	moveDB(t, path, root)

	if _, err := hub.Register(root, "kw", registry, filepath.Join(dir, "meta.db")); err != nil {
		t.Fatal(err)
	}
	rows, err := hub.Across(hub.AcrossOptions{
		Query: "similar-feedback", Keyword: "unique-keyword-token", Registry: registry,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows=%v", rows)
	}
}
