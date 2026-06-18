package importer

import (
	"path/filepath"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/migrate"
	"github.com/sebihoermann/devdb-go/internal/storage"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestImportPythonDBSamePath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy.db")
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE schema_migrations (version INTEGER, description TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO schema_migrations(version, description) VALUES (1, 'python')`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	_, err = ImportPythonDB(path, path, false)
	if err == nil {
		t.Fatal("expected same-path error")
	}
}

func TestCountParityClosedDB(t *testing.T) {
	db := testutil.ClosedDB(t)
	if _, err := CountParity(db); err == nil {
		t.Fatal("expected error")
	}
}

func TestCountGoRowsEmptyGoDB(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty-go.db")
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrate.RunAll(db); err != nil {
		t.Fatal(err)
	}
	db.Close()
	n, err := countGoRows(path)
	if err != nil || n != 0 {
		t.Fatalf("rows=%d err=%v", n, err)
	}
}

func TestInspectPythonDBClosedAfterOpen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.db")
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	if _, err := InspectPythonDB(path); err == nil {
		t.Fatal("expected inspect error for non-python file")
	}
}
