package hub_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/domain/hub"
	"github.com/sebihoermann/devdb-go/internal/domain/planning"
	"github.com/sebihoermann/devdb-go/internal/domain/verification"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestResolvePaths(t *testing.T) {
	t.Setenv("DEVDB_METADATA_DB", "")
	flagMeta := filepath.Join(t.TempDir(), "custom-meta.db")
	if got := hub.ResolveMetadataDB(flagMeta); got != flagMeta {
		t.Fatalf("metadata=%q", got)
	}
	flagReg := filepath.Join(t.TempDir(), "custom-reg")
	if got := hub.ResolveRegistry(flagReg); got != flagReg {
		t.Fatalf("registry=%q", got)
	}
}

func TestResolveMetadataDBEnvAndDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	envMeta := filepath.Join(home, "env-meta.db")
	t.Setenv("DEVDB_METADATA_DB", envMeta)
	if got := hub.ResolveMetadataDB(""); got != envMeta {
		t.Fatalf("env metadata=%q want %q", got, envMeta)
	}
	t.Setenv("DEVDB_METADATA_DB", "")
	wantDefault := filepath.Join(home, ".devdb", "metadata.db")
	if got := hub.ResolveMetadataDB(""); got != wantDefault {
		t.Fatalf("default metadata=%q want %q", got, wantDefault)
	}
	wantRegistry := filepath.Join(home, ".devdb-projects")
	if got := hub.ResolveRegistry(""); got != wantRegistry {
		t.Fatalf("default registry=%q want %q", got, wantRegistry)
	}
}

func TestRegisterExpandHomePath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := t.TempDir()
	registry := filepath.Join(dir, "registry")
	metaDB := filepath.Join(dir, "metadata.db")
	root := filepath.Join(home, "tilde-proj")
	if err := os.MkdirAll(filepath.Join(root, ".devdb"), 0o755); err != nil {
		t.Fatal(err)
	}
	db, path := testutil.TempDB(t)
	db.Close()
	if err := os.Rename(path, filepath.Join(root, ".devdb", "development.db")); err != nil {
		t.Fatal(err)
	}
	p, err := hub.Register("~/tilde-proj", "", registry, metaDB)
	if err != nil {
		t.Fatal(err)
	}
	if p.Root != root {
		t.Fatalf("root=%q want %q", p.Root, root)
	}
	if p.Alias != "tilde-proj" {
		t.Fatalf("alias=%q", p.Alias)
	}
}

func TestReadRegistryExpandHomeEntry(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := filepath.Join(home, "registry-proj")
	if err := os.MkdirAll(filepath.Join(root, ".devdb"), 0o755); err != nil {
		t.Fatal(err)
	}
	db, path := testutil.TempDB(t)
	db.Close()
	if err := os.Rename(path, filepath.Join(root, ".devdb", "development.db")); err != nil {
		t.Fatal(err)
	}
	regPath := filepath.Join(home, ".devdb-projects")
	if err := os.WriteFile(regPath, []byte("~/registry-proj custom-alias\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	projects, err := hub.ReadRegistry(regPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 || projects[0].Alias != "custom-alias" || projects[0].Root != root {
		t.Fatalf("projects=%+v", projects)
	}
}

func TestAcrossHygieneSeverityRank(t *testing.T) {
	dir := t.TempDir()
	registry := filepath.Join(dir, "registry")
	root := filepath.Join(dir, "proj")
	db, path := testutil.TempDB(t)
	_, err := db.Exec(`INSERT INTO review_runs(id, scope_paths, tier, started_at, model_id) VALUES ('r1','.','default',datetime('now'),'test')`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO review_findings(
		id, run_id, principle, title, recommendation, severity, confidence, effort, status, created_at, model_id
	) VALUES ('f1','r1','dry','critical dup','fix','critical','high','small','open',datetime('now'),'test'),
	          ('f2','r1','kiss','high smell','fix','high','high','small','open',datetime('now'),'test')`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	devdbDir := filepath.Join(root, ".devdb")
	if err := os.MkdirAll(devdbDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, filepath.Join(devdbDir, "development.db")); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.Register(root, "proj", registry, filepath.Join(dir, "meta.db")); err != nil {
		t.Fatal(err)
	}
	rows, err := hub.Across(hub.AcrossOptions{Query: "code-hygiene-cross", Registry: registry})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows=%d", len(rows))
	}
	if rows[0]["severity"] != "critical" {
		t.Fatalf("expected critical first, got %v", rows[0]["severity"])
	}
}

func TestSnapshotVerificationAndBlockedWork(t *testing.T) {
	dir := t.TempDir()
	registry := filepath.Join(dir, "registry")
	metaDB := filepath.Join(dir, "metadata.db")
	root := filepath.Join(dir, "proj")
	db, path := testutil.TempDB(t)

	planID, _ := planning.CreatePlan(db, planning.CreatePlanInput{Slug: "p", Title: "P", ModelID: "test"})
	itemID, _ := planning.AddItem(db, planning.AddItemInput{PlanID: planID, Title: "blocked task", ModelID: "test"})
	_, _ = planning.StartItem(db, itemID, "test")
	_, _ = planning.PauseItem(db, itemID, "blocked: waiting on API", "test")

	exit := 0
	runID, _ := verification.RecordRun(db, "go test ./...", ".", "", "passed", &exit, "", "", "test")
	_ = verification.FinishRun(db, runID, "passed", &exit, "")

	db.Close()
	devdbDir := filepath.Join(root, ".devdb")
	if err := os.MkdirAll(devdbDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, filepath.Join(devdbDir, "development.db")); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.Register(root, "proj", registry, metaDB); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.SyncAll(registry, metaDB, false); err != nil {
		t.Fatal(err)
	}
	detail, err := hub.Project(registry, metaDB, "proj")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Snapshot.BlockedReason == "" {
		t.Fatalf("expected blocked reason in snapshot: %+v", detail.Snapshot)
	}
	if detail.Snapshot.LatestVerificationStatus != "passed" {
		t.Fatalf("verification status=%q", detail.Snapshot.LatestVerificationStatus)
	}
}

func TestOpenHub(t *testing.T) {
	dir := t.TempDir()
	metaDB := filepath.Join(dir, "metadata.db")
	db, err := hub.OpenHub(metaDB)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}
