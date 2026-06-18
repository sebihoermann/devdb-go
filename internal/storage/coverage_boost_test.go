package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenOnDirectoryFails(t *testing.T) {
	dir := t.TempDir()
	_, err := Open(dir)
	if err == nil {
		t.Fatal("expected open error on directory path")
	}
}

func TestOpenOnReadonlyParentFails(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "nested")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Skip(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	_, err := Open(filepath.Join(sub, "blocked.db"))
	if err == nil {
		t.Skip("sqlite created db despite read-only parent on this platform")
	}
}

func TestDetectSchemaMalformedVersion(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "malformed.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE schema_migrations (version INTEGER, description TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO schema_migrations(version, description) VALUES ('not-int', 'bad')`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := DetectSchema(db); err == nil {
		t.Fatal("expected scan error for malformed version")
	}
}

func TestColumnNamesEmptyTable(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "empty.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cols, err := ColumnNames(db, "no_such_table")
	if err != nil || len(cols) != 0 {
		t.Fatalf("cols=%v err=%v", cols, err)
	}
}

func TestNewIDManyCalls(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 20; i++ {
		id, err := NewID()
		if err != nil || len(id) != 32 {
			t.Fatalf("id=%q err=%v", id, err)
		}
		if seen[id] {
			t.Fatal("duplicate id")
		}
		seen[id] = true
	}
}
