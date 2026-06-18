package hub

import (
	"testing"

	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestErrorPathsClosedHubDB(t *testing.T) {
	db := testutil.ClosedDB(t)
	ok, msg := SyncOne(db, RegisteredProject{Alias: "x", Root: t.TempDir(), DBPath: "nope", Exists: true})
	if ok {
		t.Fatalf("expected sync failure, msg=%q", msg)
	}
}
