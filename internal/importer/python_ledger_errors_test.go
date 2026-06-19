package importer_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/importer"
	"github.com/sebihoermann/devdb-go/internal/storage"
)

func TestApplyInPlaceImportFailureOnReadOnlyDir(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("chmod-as-user semantics don't apply under root")
	}
	dir := t.TempDir()
	dbPath := createMinimalPythonDB(t, dir, "development.db")
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	_, err := importer.ApplyInPlace(dbPath, true, false)
	if err == nil {
		t.Fatal("expected import failure on read-only directory")
	}
}

func TestImportRemoveDestinationFailure(t *testing.T) {
	dir := t.TempDir()
	src := createMinimalPythonDB(t, dir, "src.db")
	dst := filepath.Join(dir, "dest.db")
	if err := os.WriteFile(dst, []byte("placeholder"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	_, err := importer.ImportPythonDB(src, dst, true)
	if err == nil {
		t.Fatal("expected remove/replace failure on read-only directory")
	}
}

func TestApplyInPlaceBackupRenameFailure(t *testing.T) {
	dir := t.TempDir()
	dbPath := createMinimalPythonDB(t, dir, "development.db")
	backup := filepath.Join(dir, "development.db.python-bak")
	if err := os.Mkdir(backup, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(backup, "blocker"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := importer.ApplyInPlace(dbPath, true, false)
	if err == nil {
		t.Fatal("expected backup rename failure when backup path is a directory")
	}
}

func TestCopyLegacyDataSchemaMismatch(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "bad-legacy.db")
	db, err := storage.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	stmts := []string{
		`CREATE TABLE schema_migrations (version INTEGER, description TEXT)`,
		`INSERT INTO schema_migrations(version, description) VALUES (1, 'python init')`,
		`CREATE TABLE goals (id TEXT PRIMARY KEY, title TEXT, status TEXT, created_at TEXT, model_id TEXT)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()

	dst := filepath.Join(dir, "go.db")
	_, err = importer.ImportPythonDB(src, dst, true)
	if err == nil {
		t.Fatal("expected copy failure for mismatched legacy schema")
	}
}
