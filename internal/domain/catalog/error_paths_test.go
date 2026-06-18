package catalog

import (
	"testing"

	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestErrorPathsClosedDB(t *testing.T) {
	db := testutil.ClosedDB(t)
	if _, err := ListRows(db, "tasks", 0); err == nil {
		t.Fatal("list")
	}
	if _, err := ShowRow(db, "tasks", "x"); err == nil {
		t.Fatal("show")
	}
}
