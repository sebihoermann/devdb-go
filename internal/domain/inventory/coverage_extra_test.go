package inventory

import (
	"testing"

	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestErrorPathsClosedDBExtended(t *testing.T) {
	db := testutil.ClosedDB(t)
	repo := t.TempDir()
	if _, err := DiffSince(db, repo, "HEAD"); err == nil {
		t.Fatal("diff")
	}
	if _, err := FreshnessInfo(db); err == nil {
		t.Fatal("freshness")
	}
	payload, err := Context(db, ContextOptions{Files: []string{"main.go"}})
	if err == nil && len(payload.Files) > 0 {
		t.Fatal("context should fail on closed db")
	}
	_ = payload
}
