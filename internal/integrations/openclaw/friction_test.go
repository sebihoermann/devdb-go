package openclaw

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/domain/feedback"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func writeDaily(t *testing.T, workspace, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(workspace, "memory"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "memory", "2026-06-22.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScanFrictionCreatesStableSourceIdentity(t *testing.T) {
	db, _ := testutil.TempDB(t)
	workspace := t.TempDir()
	writeDaily(t, workspace, "# Day\n- friction: sync was noisy\n- lesson learned: keep ids stable\n")
	result, err := ScanFriction(db, workspace, nil, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Created != 2 || result.Hits != 2 {
		t.Fatalf("result=%+v", result)
	}
	rows, err := feedback.List(db, "open", 0)
	if err != nil || len(rows) != 2 {
		t.Fatalf("rows=%d err=%v", len(rows), err)
	}
	for _, row := range rows {
		if !strings.Contains(row.Note, "[src: memory/2026-06-22.md:") || !strings.Contains(row.Context, "openclaw-friction:") || !strings.Contains(row.Context, "openclaw:memory/2026-06-22.md#L") {
			t.Fatalf("row=%+v", row)
		}
	}
}

func TestScanFrictionDeduplicatesAfterLineMoves(t *testing.T) {
	db, _ := testutil.TempDB(t)
	workspace := t.TempDir()
	writeDaily(t, workspace, "friction: stable identity\n")
	first, err := ScanFriction(db, workspace, []string{"friction:"}, "test")
	if err != nil || first.Created != 1 {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	writeDaily(t, workspace, "new unrelated line\nfriction: stable identity\n")
	second, err := ScanFriction(db, workspace, []string{"friction:"}, "test")
	if err != nil || second.Created != 0 || second.Skipped != 1 {
		t.Fatalf("second=%+v err=%v", second, err)
	}
}

func TestChangedOrRemovedMarkersDoNotRewriteFeedback(t *testing.T) {
	db, _ := testutil.TempDB(t)
	if _, err := feedback.Add(db, feedback.AddInput{Role: "user", Note: "unrelated", ModelID: "test"}); err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	writeDaily(t, workspace, "broken: first wording\n")
	if _, err := ScanFriction(db, workspace, []string{"broken:"}, "test"); err != nil {
		t.Fatal(err)
	}
	writeDaily(t, workspace, "broken: changed wording\n")
	changed, err := ScanFriction(db, workspace, []string{"broken:"}, "test")
	if err != nil || changed.Created != 1 {
		t.Fatalf("changed=%+v err=%v", changed, err)
	}
	writeDaily(t, workspace, "all clear\n")
	removed, err := ScanFriction(db, workspace, []string{"broken:"}, "test")
	if err != nil || removed.Created != 0 {
		t.Fatalf("removed=%+v err=%v", removed, err)
	}
	rows, err := feedback.List(db, "open", 0)
	if err != nil || len(rows) != 3 {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
	if rows[len(rows)-1].Note != "unrelated" {
		t.Fatalf("unrelated feedback changed: %+v", rows)
	}
}
