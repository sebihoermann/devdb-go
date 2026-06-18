package migrate

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/storage"
)

func tempMigratedDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := storage.Open(filepath.Join(dir, "development.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := RunAll(db); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestRunAllMigrationApplyFailure(t *testing.T) {
	backup := SourceMigrations
	t.Cleanup(func() { SourceMigrations = backup })
	db := tempMigratedDB(t)
	defer db.Close()
	SourceMigrations = append(append([]Migration(nil), backup...), Migration{
		Version:     999991,
		Description: "test:apply-fail",
		Apply:       func(tx *sql.Tx) error { return errors.New("apply failed") },
	})
	if err := RunAll(db); err == nil {
		t.Fatal("expected migration apply error")
	}
}

func TestRunHubMigrationApplyFailure(t *testing.T) {
	backup := HubMigrations
	t.Cleanup(func() { HubMigrations = backup })
	dir := t.TempDir()
	db, err := storage.Open(filepath.Join(dir, "hub.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := RunHub(db); err != nil {
		t.Fatal(err)
	}
	HubMigrations = append(append([]Migration(nil), backup...), Migration{
		Version:     999992,
		Description: "test:hub-apply-fail",
		Apply:       func(tx *sql.Tx) error { return errors.New("hub apply failed") },
	})
	if err := RunHub(db); err == nil {
		t.Fatal("expected hub migration apply error")
	}
}

func TestRunAllInsertMigrationFailure(t *testing.T) {
	backup := SourceMigrations
	t.Cleanup(func() { SourceMigrations = backup })
	db := tempMigratedDB(t)
	defer db.Close()
	SourceMigrations = append(append([]Migration(nil), backup...), Migration{
		Version:     999993,
		Description: "test:insert-fail",
		Apply: func(tx *sql.Tx) error {
			_, err := tx.Exec(`DROP TABLE schema_migrations`)
			return err
		},
	})
	if err := RunAll(db); err == nil {
		t.Fatal("expected insert migration error")
	}
}

func TestRunHubInsertMigrationFailure(t *testing.T) {
	backup := HubMigrations
	t.Cleanup(func() { HubMigrations = backup })
	dir := t.TempDir()
	db, err := storage.Open(filepath.Join(dir, "hub2.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := RunHub(db); err != nil {
		t.Fatal(err)
	}
	HubMigrations = append(append([]Migration(nil), backup...), Migration{
		Version:     999994,
		Description: "test:hub-insert-fail",
		Apply: func(tx *sql.Tx) error {
			_, err := tx.Exec(`DROP TABLE schema_migrations`)
			return err
		},
	})
	if err := RunHub(db); err == nil {
		t.Fatal("expected hub insert migration error")
	}
}

func TestExecStatementsFailureBoost(t *testing.T) {
	dir := t.TempDir()
	db, err := storage.Open(filepath.Join(dir, "exec.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := execStatements(tx, "NOT VALID SQL;"); err == nil {
		t.Fatal("expected exec error")
	}
}
