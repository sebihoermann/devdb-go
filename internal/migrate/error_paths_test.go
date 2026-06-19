package migrate

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/storage"
)

func TestSplitStatementsSkipsEmptyAndHandlesQuotes(t *testing.T) {
	script := `CREATE TABLE t1 (note TEXT); INSERT INTO t1 VALUES ('a;b'); CREATE TABLE t2 (id INTEGER)`
	parts := splitStatements(script)
	if len(parts) < 2 {
		t.Fatalf("parts=%v", parts)
	}
	found := false
	for _, p := range parts {
		if strings.Contains(p, "a;b") {
			found = true
		}
	}
	if !found {
		t.Fatalf("quoted semicolon lost: %v", parts)
	}
}

func TestExecStatementsSkipsBlankParts(t *testing.T) {
	dir := t.TempDir()
	db, err := storage.Open(filepath.Join(dir, "split.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := execStatements(tx, "  ; CREATE TABLE blank_skip (id INTEGER);  "); err != nil {
		t.Fatal(err)
	}
	_ = tx.Rollback()
}

func closedDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := storage.Open(filepath.Join(dir, "closed.db"))
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	return db
}

func TestRunAllClosedDB(t *testing.T) {
	db := closedDB(t)
	if err := RunAll(db); err == nil {
		t.Fatal("expected error")
	}
}

func TestRunHubClosedDB(t *testing.T) {
	db := closedDB(t)
	if err := RunHub(db); err == nil {
		t.Fatal("expected error")
	}
}

func TestRunAllRowsErrPath(t *testing.T) {
	dir := t.TempDir()
	db, err := storage.Open(filepath.Join(dir, "rows.db"))
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
	if err := RunAll(db); err == nil {
		t.Fatal("expected scan error")
	}
}

func TestRunHubCommitFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("chmod-as-user semantics don't apply under root")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "hub.db")
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := RunHub(db); err != nil {
		t.Fatal(err)
	}
	backup := HubMigrations
	t.Cleanup(func() { HubMigrations = backup })
	HubMigrations = append(append([]Migration(nil), HubMigrations...), Migration{
		Version:     9996,
		Description: "test:noop",
		Apply:       func(tx *sql.Tx) error { return nil },
	})
	if _, err := db.Exec(`DELETE FROM schema_migrations WHERE version=(SELECT MAX(version) FROM schema_migrations)`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if err := os.Chmod(path, 0o444); err != nil {
		t.Skip(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o666) })
	db, err = storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := RunHub(db); err == nil {
		t.Fatal("expected commit failure on readonly db")
	}
}
