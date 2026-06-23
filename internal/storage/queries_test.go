package storage

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/migrate"
)

func TestDetectSchemaUnknown(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "empty.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	kind, ver, err := DetectSchema(db)
	if err != nil {
		t.Fatal(err)
	}
	if kind != SchemaUnknown || ver != 0 {
		t.Fatalf("got %s v%d", kind, ver)
	}
}

func TestDetectSchemaGo(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "go.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := migrate.RunAll(db); err != nil {
		t.Fatal(err)
	}
	kind, ver, err := DetectSchema(db)
	if err != nil {
		t.Fatal(err)
	}
	if kind != SchemaGo || ver == 0 {
		t.Fatalf("got %s v%d", kind, ver)
	}
}

func TestDetectSchemaPython(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "py.db"))
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
	kind, ver, err := DetectSchema(db)
	if err != nil {
		t.Fatal(err)
	}
	if kind != SchemaPython || ver != 1 {
		t.Fatalf("got %s v%d", kind, ver)
	}
}

func TestTableExistsAndColumnNames(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := migrate.RunAll(db); err != nil {
		t.Fatal(err)
	}
	ok, err := TableExists(db, "feedback")
	if err != nil || !ok {
		t.Fatalf("feedback table: ok=%v err=%v", ok, err)
	}
	ok, err = TableExists(db, "not_a_table")
	if err != nil || ok {
		t.Fatalf("missing table: ok=%v err=%v", ok, err)
	}
	cols, err := ColumnNames(db, "feedback")
	if err != nil {
		t.Fatal(err)
	}
	if len(cols) == 0 {
		t.Fatal("expected columns")
	}
	if _, err := ColumnNames(db, `feedback); DROP TABLE feedback; --`); err == nil || !strings.Contains(err.Error(), "invalid table identifier") {
		t.Fatalf("unexpected err=%v", err)
	}
}

func TestAppendLimit(t *testing.T) {
	q, args := AppendLimit("SELECT 1", nil, 0)
	if q != "SELECT 1" || len(args) != 0 {
		t.Fatalf("limit 0: q=%q args=%v", q, args)
	}
	q, args = AppendLimit("SELECT 1", []any{"x"}, 5)
	if q != "SELECT 1 LIMIT ?" || len(args) != 2 {
		t.Fatalf("limit 5: q=%q args=%v", q, args)
	}
}
