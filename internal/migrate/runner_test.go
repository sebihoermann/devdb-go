package migrate_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/migrate"
	"github.com/sebihoermann/devdb-go/internal/storage"
)

func TestRunHubFreshDB(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "metadata.db")
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := migrate.RunHub(db); err != nil {
		t.Fatal(err)
	}
	ok, err := storage.TableExists(db, "projects")
	if err != nil || !ok {
		t.Fatalf("projects table missing: %v", err)
	}
	// idempotent
	if err := migrate.RunHub(db); err != nil {
		t.Fatal(err)
	}
}

func TestRunAllMigrationFailureRollsBack(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "development.db")
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// schema_migrations exists but is corrupted to force scan error path
	if _, err := db.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO schema_migrations(version) VALUES (1)`); err != nil {
		t.Fatal(err)
	}
	// RunAll should still proceed - missing columns might cause issues on insert only
	// Test begin failure with closed db
	db.Close()
	if err := migrate.RunAll(db); err == nil {
		t.Fatal("expected error on closed db")
	}
}

func TestRunAllPartialAppliedState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "development.db")
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := migrate.RunAll(db); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != len(migrate.SourceMigrations) {
		t.Fatalf("migrations=%d want %d", count, len(migrate.SourceMigrations))
	}
}

func TestRunHubOnClosedDB(t *testing.T) {
	dir := t.TempDir()
	db, err := storage.Open(filepath.Join(dir, "meta.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	if err := migrate.RunHub(db); err == nil {
		t.Fatal("expected error on closed db")
	}
}

func TestRunAllSchemaMigrationsInsertFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "development.db")
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if err := migrate.RunAll(db); err == nil {
		t.Fatal("expected migration insert failure on incomplete schema_migrations table")
	}
}

func TestRunHubSchemaMigrationsInsertFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "metadata.db")
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if err := migrate.RunHub(db); err == nil {
		t.Fatal("expected hub migration insert failure on incomplete schema_migrations table")
	}
}

func TestRunAllResumePendingMigrations(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "development.db")
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := migrate.RunAll(db); err != nil {
		t.Fatal(err)
	}
	var maxVersion int
	if err := db.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&maxVersion); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM schema_migrations WHERE version=?`, maxVersion); err != nil {
		t.Fatal(err)
	}
	if err := migrate.RunAll(db); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != len(migrate.SourceMigrations) {
		t.Fatalf("migrations=%d want %d", count, len(migrate.SourceMigrations))
	}
}

func TestRunHubResumePendingMigrations(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "metadata.db")
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := migrate.RunHub(db); err != nil {
		t.Fatal(err)
	}
	var maxVersion int
	if err := db.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&maxVersion); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM schema_migrations WHERE version=?`, maxVersion); err != nil {
		t.Fatal(err)
	}
	if err := migrate.RunHub(db); err != nil {
		t.Fatal(err)
	}
}

func TestRunAllScanErrorOnSchemaMigrations(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "development.db")
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE schema_migrations (version TEXT PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO schema_migrations(version) VALUES ('bad')`); err != nil {
		t.Fatal(err)
	}
	if err := migrate.RunAll(db); err == nil {
		t.Fatal("expected scan error")
	}
}

func TestRunHubScanErrorOnSchemaMigrations(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "metadata.db")
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE schema_migrations (version TEXT PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO schema_migrations(version) VALUES ('bad')`); err != nil {
		t.Fatal(err)
	}
	if err := migrate.RunHub(db); err == nil {
		t.Fatal("expected hub scan error")
	}
}

func TestRunAllFailsOnReadonlyDatabase(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("chmod-as-user semantics don't apply under root")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "development.db")
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrate.RunAll(db); err != nil {
		t.Fatal(err)
	}
	var maxVersion int
	if err := db.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&maxVersion); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM schema_migrations WHERE version=?`, maxVersion); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o666) })
	db, err = storage.Open(path + "?mode=ro")
	if err != nil {
		db, err = storage.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		if err := migrate.RunAll(db); err != nil {
			return
		}
		t.Fatal("expected error when re-applying removed migration")
	}
	defer db.Close()
	if err := migrate.RunAll(db); err == nil {
		t.Fatal("expected migration failure on readonly database")
	}
}

// ensure sql.DB type used
var _ *sql.DB
