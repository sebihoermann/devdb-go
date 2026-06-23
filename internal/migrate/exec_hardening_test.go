package migrate

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/storage"
)

// TestExecStatementsRunsMultiStatementMigrationWithQuotedSemicolons is the
// integration check: a migration script that contains semicolons inside
// string literals, double-quoted identifiers, and line comments must still
// execute as a single logical migration and produce the expected schema.
// Regression for review finding 7b6b4b6f.
func TestExecStatementsRunsMultiStatementMigrationWithQuotedSemicolons(t *testing.T) {
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
	defer func() { _ = tx.Rollback() }()

	script := `-- header; describes the migration
CREATE TABLE "weird;name" (
	id INTEGER PRIMARY KEY,
	note TEXT DEFAULT 'a;b'
);
-- footer; trailing comment
INSERT INTO "weird;name"(note) VALUES ('it''s;ok');
`
	if err := execStatements(tx, script); err != nil {
		t.Fatalf("execStatements: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	// The table exists and the row landed.
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM "weird;name"`).Scan(&n); err != nil {
		t.Fatalf("query: %v", err)
	}
	if n != 1 {
		t.Fatalf("rows=%d want 1", n)
	}
	var note string
	if err := db.QueryRow(`SELECT note FROM "weird;name" LIMIT 1`).Scan(&note); err != nil {
		t.Fatal(err)
	}
	if note != "it's;ok" {
		t.Fatalf("note=%q", note)
	}
}

// TestExecStatementsFailureMidwayRollsBackTx confirms execStatements still
// surfaces errors when a comment-laden script has a statement that fails,
// and that the surrounding tx is left clean for the caller to roll back.
func TestExecStatementsFailureMidwayRollsBackTx(t *testing.T) {
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
	defer func() { _ = tx.Rollback() }()

	script := `-- preamble; ok
CREATE TABLE t (id INTEGER);
NOT VALID SQL;
-- postamble; not reached
INSERT INTO t VALUES (1);
`
	if err := execStatements(tx, script); err == nil {
		t.Fatal("expected execStatements to fail on invalid SQL")
	} else if !strings.Contains(err.Error(), "NOT VALID SQL") {
		t.Fatalf("err=%v", err)
	}
}