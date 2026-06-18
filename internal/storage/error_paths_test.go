package storage

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func closedDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "closed.db"))
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	return db
}

func TestDetectSchemaClosedDB(t *testing.T) {
	db := closedDB(t)
	if _, _, err := DetectSchema(db); err == nil {
		t.Fatal("expected error")
	}
}

func TestTableExistsClosedDB(t *testing.T) {
	db := closedDB(t)
	if _, err := TableExists(db, "goals"); err == nil {
		t.Fatal("expected error")
	}
}

func TestColumnNamesClosedDB(t *testing.T) {
	db := closedDB(t)
	if _, err := ColumnNames(db, "goals"); err == nil {
		t.Fatal("expected error")
	}
}

func TestWithTxBeginFailure(t *testing.T) {
	db := closedDB(t)
	err := WithTx(db, func(tx *sql.Tx) error { return nil })
	if err == nil {
		t.Fatal("expected begin error")
	}
}

func TestWithTxCommitFailure(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "commit.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := WithTx(db, func(tx *sql.Tx) error {
		_, err := tx.Exec(`CREATE TABLE once (id INTEGER)`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	db.Close()
	err = WithTx(db, func(tx *sql.Tx) error { return nil })
	if err == nil {
		t.Fatal("expected commit error on closed db")
	}
}

func TestDetectSchemaPythonBranch(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "python.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE schema_migrations (version INTEGER, description TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO schema_migrations(version, description) VALUES (3, 'python legacy')`); err != nil {
		t.Fatal(err)
	}
	kind, ver, err := DetectSchema(db)
	if err != nil || kind != SchemaPython || ver != 3 {
		t.Fatalf("kind=%s ver=%d err=%v", kind, ver, err)
	}
}

func TestColumnNamesScanError(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "bad.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	if _, err := ColumnNames(db, "missing"); err == nil {
		t.Fatal("expected query error")
	}
}

func TestWithTxFnError(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "rollback.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	want := errors.New("fn failed")
	if err := WithTx(db, func(tx *sql.Tx) error {
		return want
	}); !errors.Is(err, want) {
		t.Fatalf("err=%v", err)
	}
}
