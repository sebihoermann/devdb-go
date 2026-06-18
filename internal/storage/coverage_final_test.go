package storage

import (
	"path/filepath"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/migrate"
)

func TestResolveIDBranches(t *testing.T) {
	ids := []string{"aaa1111111111111111111111111111", "aaa2222222222222222222222222222"}
	if _, err := ResolveID("", ids); err == nil {
		t.Fatal("empty prefix")
	}
	got, err := ResolveID("aaa11111", ids)
	if err != nil || got != ids[0] {
		t.Fatalf("prefix=%q err=%v", got, err)
	}
	full, err := ResolveID(ids[1], ids)
	if err != nil || full != ids[1] {
		t.Fatalf("full=%q err=%v", full, err)
	}
	_, err = ResolveID("aaa", ids)
	if err == nil {
		t.Fatal("expected ambiguous")
	}
	_, err = ResolveID("zzzz", ids)
	if err == nil {
		t.Fatal("expected no match")
	}
}

func TestDetectSchemaBranches(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "empty.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	kind, ver, err := DetectSchema(db)
	if err != nil || kind != SchemaUnknown || ver != 0 {
		t.Fatalf("empty: kind=%s ver=%d err=%v", kind, ver, err)
	}
	if err := migrate.RunAll(db); err != nil {
		t.Fatal(err)
	}
	kind, ver, err = DetectSchema(db)
	if err != nil || kind != SchemaGo || ver == 0 {
		t.Fatalf("go schema: kind=%s ver=%d err=%v", kind, ver, err)
	}
}

func TestColumnNamesAndAppendLimit(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "cols.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE t (id INTEGER, name TEXT)`); err != nil {
		t.Fatal(err)
	}
	cols, err := ColumnNames(db, "t")
	if err != nil || len(cols) != 2 {
		t.Fatalf("cols=%v err=%v", cols, err)
	}
	q, args := AppendLimit("SELECT 1", nil, 0)
	if q != "SELECT 1" || len(args) != 0 {
		t.Fatalf("no limit: %q %v", q, args)
	}
	q, args = AppendLimit("SELECT 1", nil, 5)
	if q != "SELECT 1 LIMIT ?" || len(args) != 1 {
		t.Fatalf("limit: %q %v", q, args)
	}
	exists, err := TableExists(db, "t")
	if err != nil || !exists {
		t.Fatalf("exists=%v err=%v", exists, err)
	}
}

func TestNewIDUnique(t *testing.T) {
	a, err := NewID()
	if err != nil || len(a) != 32 {
		t.Fatalf("id=%q err=%v", a, err)
	}
	b, _ := NewID()
	if a == b {
		t.Fatal("ids should differ")
	}
}
