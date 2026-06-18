package goals

import (
	"testing"

	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestNullIfEmptyAndResolveID(t *testing.T) {
	db, _ := testutil.TempDB(t)
	id, _ := Add(db, "goal", "Title", "", "test")
	if v := nullIfEmpty(""); v != nil {
		t.Fatal("empty should be nil")
	}
	got, err := resolveID(db, id[:8])
	if err != nil || got != id {
		t.Fatalf("resolve=%q err=%v", got, err)
	}
	_, err = SetStatus(db, id, "bogus", "test")
	if err == nil {
		t.Fatal("bad status")
	}
	rows, _ := List(db, "done", 0)
	_ = rows
}
