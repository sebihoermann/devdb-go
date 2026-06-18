package migrate

import (
	"path/filepath"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/storage"
)

func TestSplitStatementsQuotedSemicolon(t *testing.T) {
	script := `INSERT INTO t VALUES ('a;b'); INSERT INTO t VALUES (1);`
	parts := splitStatements(script)
	if len(parts) != 2 {
		t.Fatalf("parts=%v", parts)
	}
}

func TestExecStatementsInvalidSQL(t *testing.T) {
	dir := t.TempDir()
	db, err := storage.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := execStatements(tx, `NOT VALID SQL;`); err == nil {
		t.Fatal("expected exec error")
	}
	_ = tx.Rollback()
}

func TestRunAllRowsScanError(t *testing.T) {
	dir := t.TempDir()
	db, err := storage.Open(filepath.Join(dir, "t.db"))
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
	if err := RunAll(db); err == nil {
		t.Fatal("expected scan error from bad migration version type")
	}
}

func TestRunHubMigrationFailure(t *testing.T) {
	dir := t.TempDir()
	db, err := storage.Open(filepath.Join(dir, "meta.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE schema_migrations (version TEXT PRIMARY KEY, applied_at TEXT, description TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO schema_migrations(version, applied_at, description) VALUES ('x', 'now', 'bad')`); err != nil {
		t.Fatal(err)
	}
	if err := RunHub(db); err == nil {
		t.Fatal("expected hub migration failure")
	}
}
