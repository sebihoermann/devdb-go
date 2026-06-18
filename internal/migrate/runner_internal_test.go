package migrate

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/storage"
)

func TestExecStatementsFailure(t *testing.T) {
	dir := t.TempDir()
	db, err := storage.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := execStatements(tx, "NOT VALID SQL"); err == nil {
		t.Fatal("expected execStatements error")
	}
	_ = tx.Rollback()
}

func TestRunAllMigrationApplyRollback(t *testing.T) {
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
	backup := SourceMigrations
	t.Cleanup(func() { SourceMigrations = backup })
	SourceMigrations = append(append([]Migration(nil), SourceMigrations...), Migration{
		Version:     9999,
		Description: "test:forced failure",
		Apply: func(tx *sql.Tx) error {
			return fmt.Errorf("forced apply failure")
		},
	})
	if err := RunAll(db); err == nil {
		t.Fatal("expected migration apply failure")
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version=9999`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("failed migration should not be recorded")
	}
}

func TestRunHubMigrationApplyRollback(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "metadata.db")
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := RunHub(db); err != nil {
		t.Fatal(err)
	}
	backup := HubMigrations
	t.Cleanup(func() { HubMigrations = backup })
	HubMigrations = append(append([]Migration(nil), HubMigrations...), Migration{
		Version:     9999,
		Description: "test:forced hub failure",
		Apply: func(tx *sql.Tx) error {
			return fmt.Errorf("forced hub apply failure")
		},
	})
	if err := RunHub(db); err == nil {
		t.Fatal("expected hub migration apply failure")
	}
}

func TestRunAllMigrationInsertFailure(t *testing.T) {
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
	backup := SourceMigrations
	t.Cleanup(func() { SourceMigrations = backup })
	SourceMigrations = append(append([]Migration(nil), SourceMigrations...), Migration{
		Version:     9998,
		Description: "test:success",
		Apply:       func(tx *sql.Tx) error { return nil },
	})
	if _, err := db.Exec(`ALTER TABLE schema_migrations RENAME TO schema_migrations_backup`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if err := RunAll(db); err == nil {
		t.Fatal("expected insert failure with incomplete schema_migrations")
	}
}

func TestRunHubMigrationInsertFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "metadata.db")
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := RunHub(db); err != nil {
		t.Fatal(err)
	}
	backup := HubMigrations
	t.Cleanup(func() { HubMigrations = backup })
	HubMigrations = append(append([]Migration(nil), HubMigrations...), Migration{
		Version:     9997,
		Description: "test:hub-success",
		Apply:       func(tx *sql.Tx) error { return nil },
	})
	if _, err := db.Exec(`ALTER TABLE schema_migrations RENAME TO schema_migrations_backup`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if err := RunHub(db); err == nil {
		t.Fatal("expected hub insert failure with incomplete schema_migrations")
	}
}
