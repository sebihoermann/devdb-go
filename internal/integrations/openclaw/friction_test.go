package openclaw

import (
	"database/sql"
	"io"
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
	result, err := ScanFriction(db, workspace, nil, "test", false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Created != 2 || result.Hits != 2 {
		t.Fatalf("result=%+v", result)
	}
	rows, err := feedback.ListByStatus(db, "open", 0)
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
	first, err := ScanFriction(db, workspace, []string{"friction:"}, "test", false)
	if err != nil || first.Created != 1 {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	writeDaily(t, workspace, "new unrelated line\nfriction: stable identity\n")
	second, err := ScanFriction(db, workspace, []string{"friction:"}, "test", false)
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
	if _, err := ScanFriction(db, workspace, []string{"broken:"}, "test", false); err != nil {
		t.Fatal(err)
	}
	writeDaily(t, workspace, "broken: changed wording\n")
	changed, err := ScanFriction(db, workspace, []string{"broken:"}, "test", false)
	if err != nil || changed.Created != 1 {
		t.Fatalf("changed=%+v err=%v", changed, err)
	}
	writeDaily(t, workspace, "all clear\n")
	removed, err := ScanFriction(db, workspace, []string{"broken:"}, "test", false)
	if err != nil || removed.Created != 0 {
		t.Fatalf("removed=%+v err=%v", removed, err)
	}
	rows, err := feedback.ListByStatus(db, "open", 0)
	if err != nil || len(rows) != 3 {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
	if rows[len(rows)-1].Note != "unrelated" {
		t.Fatalf("unrelated feedback changed: %+v", rows)
	}
}

// TestFrictionScanDryRunWritesNothing proves the preview path: it reports the
// rows it would create and leaves the ledger untouched.
func TestFrictionScanDryRunWritesNothing(t *testing.T) {
	db, _ := testutil.TempDB(t)
	workspace := t.TempDir()
	writeDaily(t, workspace, "# Day\n- friction: dry run probe\n- friction: second probe\n")
	before := len(mustListOpen(t, db))

	result, err := ScanFriction(db, workspace, []string{"friction:"}, "test-model", true)
	if err != nil {
		t.Fatal(err)
	}
	if !result.DryRun {
		t.Fatal("result not marked as a dry run")
	}
	if len(result.Proposed) == 0 {
		t.Fatal("dry run proposed nothing, expected at least one row")
	}
	if result.Created != len(result.Proposed) {
		t.Fatalf("created=%d but proposed=%d", result.Created, len(result.Proposed))
	}
	if result.Proposed[0].Role != "codebase" || result.Proposed[0].Severity != "med" {
		t.Fatalf("unexpected proposal shape: %+v", result.Proposed[0])
	}

	after := len(mustListOpen(t, db))
	if after != before {
		t.Fatalf("dry run wrote rows: %d -> %d", before, after)
	}

	// A real run then creates exactly what the preview listed.
	real, err := ScanFriction(db, workspace, []string{"friction:"}, "test-model", false)
	if err != nil {
		t.Fatal(err)
	}
	if real.Created != len(result.Proposed) {
		t.Fatalf("real run created %d, preview predicted %d", real.Created, len(result.Proposed))
	}
	if len(real.Proposed) != 0 {
		t.Fatalf("non-dry run should not populate Proposed: %+v", real.Proposed)
	}
}

// mustListOpen returns the open feedback rows, failing the test on error.
func mustListOpen(t *testing.T, db *sql.DB) []feedback.Row {
	t.Helper()
	rows, err := feedback.ListByStatus(db, "open", 0)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

// TestScheduleVerbIsGone is acceptance 1: the cron-creating verb is removed
// rather than left able to install jobs that fail.
func TestScheduleVerbIsGone(t *testing.T) {
	root := NewCommand()
	for _, cmd := range root.Commands() {
		if cmd.Name() == "schedule" {
			t.Fatal("schedule verb still registered")
		}
	}
	// Resolution of the old verb must fail, not silently succeed.
	root.SetArgs([]string{"schedule"})
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	if err := root.Execute(); err == nil {
		t.Fatal("devdb-openclaw schedule exited 0")
	}
}
