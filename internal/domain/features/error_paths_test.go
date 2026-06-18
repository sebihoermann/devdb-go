package features

import (
	"testing"

	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestErrorPathsClosedDB(t *testing.T) {
	db := testutil.ClosedDB(t)
	if _, err := Add(db, "f", "", "", "", "m"); err == nil {
		t.Fatal("add")
	}
	if _, err := List(db, 0); err == nil {
		t.Fatal("list")
	}
}
