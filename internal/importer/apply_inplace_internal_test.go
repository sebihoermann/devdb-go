package importer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/migrate"
	"github.com/sebihoermann/devdb-go/internal/storage"
)

func TestDetectFileSchemaInvalidFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.db")
	if err := os.WriteFile(path, []byte("not-a-database"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := detectFileSchema(path)
	if err == nil {
		t.Fatal("expected detect schema error")
	}
}

func TestCountGoRowsEmptyDatabase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.db")
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
		t.Fatalf("count=%d err=%v", n, err)
	}
}

func writeMinimalPythonDBWithFeedback(path string) error {
	db, err := storage.Open(path)
	if err != nil {
		return err
	}
	defer db.Close()
	stmts := []string{
		`CREATE TABLE schema_migrations (version INTEGER, description TEXT)`,
		`INSERT INTO schema_migrations(version, description) VALUES (1, 'python init')`,
		`CREATE TABLE feedback (
			id TEXT PRIMARY KEY, role TEXT NOT NULL, category TEXT, severity TEXT,
			note TEXT NOT NULL, context TEXT, status TEXT NOT NULL, proposed_fix TEXT,
			created_at TEXT NOT NULL, model_id TEXT NOT NULL)`,
		`INSERT INTO feedback(id, role, note, status, created_at, model_id)
			VALUES ('f1', 'model', 'note', 'open', '2020-01-01T00:00:00Z', 'test')`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func TestApplyInPlaceReportsRestoreFailure(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "development.db")
	if err := writeMinimalPythonDBWithFeedback(dbPath); err != nil {
		t.Fatal(err)
	}

	originalRename := renameFile
	t.Cleanup(func() { renameFile = originalRename })
	renameCalls := 0
	renameFile = func(oldPath, newPath string) error {
		renameCalls++
		switch renameCalls {
		case 2:
			return fmt.Errorf("replace failed")
		case 3:
			return fmt.Errorf("restore failed")
		default:
			return originalRename(oldPath, newPath)
		}
	}

	_, err := ApplyInPlace(dbPath, true, false)
	if err == nil {
		t.Fatal("expected replace failure")
	}
	if !strings.Contains(err.Error(), "replace database: replace failed") {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(err.Error(), "restore backup: restore failed") {
		t.Fatalf("err=%v", err)
	}
}
