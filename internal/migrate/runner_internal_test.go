package migrate

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/storage"
)

func TestValidateMigrations(t *testing.T) {
	tests := []struct {
		name       string
		migrations []Migration
		want       string
	}{
		{name: "valid", migrations: []Migration{{Version: 1}, {Version: 2}, {Version: 4}}},
		{name: "zero", migrations: []Migration{{Version: 0}}, want: "non-positive"},
		{name: "negative", migrations: []Migration{{Version: -1}}, want: "non-positive"},
		{name: "duplicate", migrations: []Migration{{Version: 1}, {Version: 2}, {Version: 2}}, want: "must be greater"},
		{name: "unordered", migrations: []Migration{{Version: 1}, {Version: 3}, {Version: 2}}, want: "must be greater"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMigrations("test", tt.migrations)
			if tt.want == "" && err != nil {
				t.Fatal(err)
			}
			if tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)) {
				t.Fatalf("error=%v want substring %q", err, tt.want)
			}
		})
	}
}

func TestRunAllRejectsDuplicateVersionBeforeDatabaseMutation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "development.db")
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	backup := SourceMigrations
	t.Cleanup(func() { SourceMigrations = backup })
	SourceMigrations = append(append([]Migration(nil), SourceMigrations...), Migration{
		Version:     SourceMigrations[len(SourceMigrations)-1].Version,
		Description: "test:duplicate memory_ref-style collision",
		Apply: func(tx *sql.Tx) error {
			_, err := tx.Exec(`CREATE TABLE should_not_exist (id INTEGER)`)
			return err
		},
	})

	err = RunAll(db)
	if err == nil || !strings.Contains(err.Error(), "must be greater") {
		t.Fatalf("error=%v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name IN ('schema_migrations','should_not_exist')`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("migration validation mutated database: tables=%d", count)
	}
}

func TestRunHubRejectsUnorderedVersionBeforeDatabaseMutation(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "metadata.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	backup := HubMigrations
	t.Cleanup(func() { HubMigrations = backup })
	HubMigrations = append(append([]Migration(nil), HubMigrations...), Migration{Version: 1, Description: "test:duplicate"})
	if err := RunHub(db); err == nil || !strings.Contains(err.Error(), "must be greater") {
		t.Fatalf("error=%v", err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='schema_migrations'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("hub validation mutated database: tables=%d", count)
	}
}

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
