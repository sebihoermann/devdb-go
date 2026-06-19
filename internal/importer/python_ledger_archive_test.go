package importer_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/importer"
	"github.com/sebihoermann/devdb-go/internal/storage"
)

// createPythonDBWithPythonOnlyTables builds a python-shaped source DB that
// includes two populated python-only tables (entity_links and loc_snapshots).
func createPythonDBWithPythonOnlyTables(t *testing.T, dir string) string {
	t.Helper()
	dbPath := filepath.Join(dir, "development.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE schema_migrations (version INTEGER, description TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO schema_migrations(version, description) VALUES (1, 'python init')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE goals (
		id TEXT PRIMARY KEY, kind TEXT NOT NULL, title TEXT NOT NULL, body TEXT,
		status TEXT NOT NULL, created_at TEXT NOT NULL, model_id TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO goals(id, kind, title, status, created_at, model_id)
		VALUES ('g1', 'goal', 'Ship', 'active', '2020-01-01T00:00:00Z', 'test')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE entity_links (
		id TEXT PRIMARY KEY, src_id TEXT NOT NULL, src_table TEXT NOT NULL,
		dst_id TEXT NOT NULL, dst_table TEXT NOT NULL, kind TEXT, note TEXT,
		created_at TEXT NOT NULL, model_id TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	for i, sid := range []string{"a", "b", "c", "d", "e"} {
		if _, err := db.Exec(`INSERT INTO entity_links(id, src_id, src_table, dst_id, dst_table, created_at, model_id)
			VALUES (?, ?, 'goals', 'g1', 'goals', '2020-01-01T00:00:00Z', 'test')`,
			"el"+string(rune('1'+i)), sid); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`CREATE TABLE loc_snapshots (
		id TEXT PRIMARY KEY, captured_at TEXT NOT NULL, language TEXT,
		total_lines INTEGER, code_lines INTEGER, files INTEGER,
		created_at TEXT NOT NULL, model_id TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO loc_snapshots(id, captured_at, total_lines, code_lines, files, created_at, model_id)
		VALUES ('loc1', '2020-01-01T00:00:00Z', 100, 80, 10, '2020-01-01T00:00:00Z', 'test')`); err != nil {
		t.Fatal(err)
	}
	return dbPath
}

func TestApplyArchivesPythonOnlyTables(t *testing.T) {
	dir := t.TempDir()
	srcPath := createPythonDBWithPythonOnlyTables(t, dir)
	result, err := importer.ApplyInPlace(srcPath, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Archived) != 2 {
		t.Fatalf("archived tables: %+v", result.Archived)
	}
	seen := map[string]int{}
	for _, a := range result.Archived {
		seen[a.Table] = a.Rows
		b, err := os.ReadFile(a.Path)
		if err != nil {
			t.Fatalf("read %s: %v", a.Table, err)
		}
		lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
		if len(lines) != a.Rows {
			t.Fatalf("%s jsonl lines=%d rows=%d", a.Table, len(lines), a.Rows)
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(lines[0]), &rec); err != nil {
			t.Fatalf("decode %s: %v", a.Table, err)
		}
	}
	if seen["entity_links"] != 5 || seen["loc_snapshots"] != 1 {
		t.Fatalf("expected entity_links=5 loc_snapshots=1, got %+v", seen)
	}
	archiveDir := filepath.Join(dir, "archive-python-only")
	for _, table := range []string{"entity_links.jsonl", "loc_snapshots.jsonl"} {
		if _, err := os.Stat(filepath.Join(archiveDir, table)); err != nil {
			t.Fatalf("expected archive file %s: %v", table, err)
		}
	}
}

func TestApplyWithNoArchiveFlagSkipsArchive(t *testing.T) {
	dir := t.TempDir()
	srcPath := createPythonDBWithPythonOnlyTables(t, dir)
	result, err := importer.ApplyInPlace(srcPath, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Archived) != 0 {
		t.Fatalf("expected empty Archived, got %+v", result.Archived)
	}
	archiveDir := filepath.Join(dir, "archive-python-only")
	if _, err := os.Stat(archiveDir); err == nil {
		t.Fatalf("archive dir should not exist: %v", err)
	}
}

func TestApplyWithEmptyPythonOnlyTables(t *testing.T) {
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "development.db")
	db, err := storage.Open(srcPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE schema_migrations (version INTEGER, description TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO schema_migrations(version, description) VALUES (1, 'python init')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE goals (
		id TEXT PRIMARY KEY, kind TEXT NOT NULL, title TEXT NOT NULL, body TEXT,
		status TEXT NOT NULL, created_at TEXT NOT NULL, model_id TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO goals(id, kind, title, status, created_at, model_id)
		VALUES ('g1', 'goal', 'Ship', 'active', '2020-01-01T00:00:00Z', 'test')`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	result, err := importer.ApplyInPlace(srcPath, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Archived) != 0 {
		t.Fatalf("expected empty Archived, got %+v", result.Archived)
	}
}
