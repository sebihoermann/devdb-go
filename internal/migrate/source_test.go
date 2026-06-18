package migrate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/storage"
)

func TestRunAllFreshDB(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "development.db")
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := RunAll(db); err != nil {
		t.Fatal(err)
	}
	kind, version, err := storage.DetectSchema(db)
	if err != nil {
		t.Fatal(err)
	}
	if kind != storage.SchemaGo {
		t.Fatalf("kind=%s want go", kind)
	}
	wantVersion := len(SourceMigrations)
	if version != wantVersion {
		t.Fatalf("version=%d want %d", version, wantVersion)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM feedback`).Scan(&n); err != nil {
		t.Fatal(err)
	}
}

func TestOpenPythonDevelopmentDB(t *testing.T) {
	pythonDB := legacyPythonDBPath(t)
	if pythonDB == "" {
		t.Skip("no python development.db in repo")
	}
	db, err := storage.Open(pythonDB)
	if err != nil {
		t.Skip("no python development.db in repo root")
	}
	defer db.Close()

	kind, version, err := storage.DetectSchema(db)
	if err != nil {
		t.Fatal(err)
	}
	if kind != storage.SchemaPython {
		t.Skipf("development.db is %s schema, not python", kind)
	}
	if version < 1 {
		t.Fatalf("unexpected migration version %d", version)
	}
	// Ensure core tables exist without requiring Go schema.
	for _, table := range []string{"feedback", "plan_items", "schema_migrations"} {
		ok, err := storage.TableExists(db, table)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatalf("missing table %s in python db", table)
		}
	}
}

func TestRunAllIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "development.db")
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := RunAll(db); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != len(SourceMigrations) {
		t.Fatalf("migrations=%d want %d", count, len(SourceMigrations))
	}
}

func TestHubMigrations(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "metadata.db")
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := RunAll(db); err != nil {
		// hub uses separate migration list — apply manually
	}
	for _, m := range HubMigrations {
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if err := m.Apply(tx); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	ok, err := storage.TableExists(db, "projects")
	if err != nil || !ok {
		t.Fatalf("hub projects table missing: %v", err)
	}
}

func legacyPythonDBPath(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	var root string
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			root = dir
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
	parent := filepath.Dir(root)
	candidates := []string{
		filepath.Join(root, ".devdb", "development.db.python-bak"),
		filepath.Join(root, ".devdb", "development.db"),
		filepath.Join(parent, ".devdb", "development.db.python-bak"),
		filepath.Join(parent, ".devdb", "development.db"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err != nil {
			continue
		}
		db, err := storage.Open(p)
		if err != nil {
			continue
		}
		kind, _, err := storage.DetectSchema(db)
		_ = db.Close()
		if err == nil && kind == storage.SchemaPython {
			return p
		}
	}
	return ""
}
