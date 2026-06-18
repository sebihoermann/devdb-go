package importer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/migrate"
	"github.com/sebihoermann/devdb-go/internal/storage"
)

func TestCountParityQueryError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "broken.db")
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE feedback (id TEXT)`); err != nil {
		t.Fatal(err)
	}
	// Close and corrupt: reopen without proper schema for joined query
	db.Close()
	db, err = storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	got, err := CountParity(db)
	if err != nil {
		// missing tables return partial counts
		if got.Feedback != 0 {
			t.Fatalf("partial=%+v err=%v", got, err)
		}
	}
}

func TestImportPythonDBInspectOpenError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "unreadable")
	if err := os.Mkdir(path, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o755) })
	_, err := ImportPythonDB(filepath.Join(path, "inside.db"), filepath.Join(t.TempDir(), "out.db"), false)
	if err == nil {
		t.Fatal("expected import error")
	}
}

func TestDetectFileSchemaGoEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "go.db")
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrate.RunAll(db); err != nil {
		t.Fatal(err)
	}
	db.Close()
	kind, _, err := detectFileSchema(path)
	if err != nil || kind != storage.SchemaGo {
		t.Fatalf("kind=%s err=%v", kind, err)
	}
}
