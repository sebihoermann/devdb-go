package dedupe_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/domain/dedupe"
	"github.com/sebihoermann/devdb-go/internal/storage"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := storage.Open(filepath.Join(t.TempDir(), "dedupe.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`CREATE TABLE notes (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		status TEXT NOT NULL
	)`); err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct{ id, title, status string }{
		{"a", "Fix  the parser", "open"},
		{"b", "Ship it", "closed"},
	} {
		if _, err := db.Exec(`INSERT INTO notes(id, title, status) VALUES (?, ?, ?)`, row.id, row.title, row.status); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func TestFindNormalizesAndFilters(t *testing.T) {
	db := openTestDB(t)
	match, err := dedupe.Find(db, dedupe.Query{
		Table: "notes", TextColumn: "title", Text: "fix the   parser",
		Filters: []dedupe.Filter{{Column: "status", Value: "open"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if match == nil || match.ID != "a" {
		t.Fatalf("match=%+v, want id a", match)
	}

	match, err = dedupe.Find(db, dedupe.Query{
		Table: "notes", TextColumn: "title", Text: "Ship it",
		Filters: []dedupe.Filter{{Column: "status", Value: "open"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if match != nil {
		t.Fatalf("closed row should not match open filter: %+v", match)
	}
}

func TestFindEmptyAndNoMatch(t *testing.T) {
	db := openTestDB(t)
	for _, text := range []string{"", "   "} {
		match, err := dedupe.Find(db, dedupe.Query{Table: "notes", TextColumn: "title", Text: text})
		if err != nil {
			t.Fatal(err)
		}
		if match != nil {
			t.Fatalf("empty text matched %+v", match)
		}
	}
	match, err := dedupe.Find(db, dedupe.Query{Table: "notes", TextColumn: "title", Text: "unrelated"})
	if err != nil {
		t.Fatal(err)
	}
	if match != nil {
		t.Fatalf("unexpected match %+v", match)
	}
}

func TestFindQueryError(t *testing.T) {
	db := openTestDB(t)
	if _, err := dedupe.Find(db, dedupe.Query{Table: "missing", TextColumn: "title", Text: "x"}); err == nil {
		t.Fatal("expected error for missing table")
	}
}
