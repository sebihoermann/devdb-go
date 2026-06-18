package migrate

import (
	"path/filepath"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/storage"
)

func TestSplitStatementsCoverage(t *testing.T) {
	script := "CREATE TABLE t1 (id INTEGER); CREATE TABLE t2 (id INTEGER);"
	parts := splitStatements(script)
	if len(parts) < 2 {
		t.Fatalf("parts=%d", len(parts))
	}
}

func TestRunAllIdempotentFresh(t *testing.T) {
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
	if err := RunAll(db); err != nil {
		t.Fatal(err)
	}
}
