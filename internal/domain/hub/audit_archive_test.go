package hub_test

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/domain/archive"
	"github.com/sebihoermann/devdb-go/internal/domain/hub"
	"github.com/sebihoermann/devdb-go/internal/storage"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

// registerLedger creates a project root holding a migrated ledger, then
// registers it in the hub so Audit can find it.
func registerLedger(t *testing.T, alias string) (root, registry, metadata string) {
	t.Helper()
	dir := t.TempDir()
	root = filepath.Join(dir, alias)
	if err := os.MkdirAll(filepath.Join(root, ".devdb"), 0o755); err != nil {
		t.Fatal(err)
	}
	db, path := testutil.TempDB(t)
	for i := 0; i < 3; i++ {
		if _, err := db.Exec(`
			INSERT INTO feedback(id, role, severity, category, note, context, status, created_at)
			VALUES (?, 'user', 'low', 'dx', 'live row', '', 'open', '2026-01-01T00:00:00Z')`,
			"fb-live-"+string(rune('a'+i))); err != nil {
			t.Fatal(err)
		}
	}
	// One closed row old enough for the sweep to archive.
	if _, err := db.Exec(`
		INSERT INTO feedback(id, role, severity, category, note, context, status, created_at)
		VALUES ('fb-closed', 'user', 'low', 'dx', 'closed row', '', 'closed', '2020-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	target := filepath.Join(root, ".devdb", "development.db")
	if err := os.Rename(path, target); err != nil {
		t.Fatal(err)
	}
	registry = filepath.Join(dir, "registry")
	metadata = filepath.Join(dir, "meta.db")
	if _, err := hub.Register(root, alias, registry, metadata); err != nil {
		t.Fatal(err)
	}
	return root, registry, metadata
}

// openLedger opens a registered project ledger read-write.
func openLedger(root string) (*sql.DB, error) {
	return storage.Open(filepath.Join(root, ".devdb", "development.db"))
}

// archiveClosedFeedback runs the sweep against the registered ledger so the
// archived side of the report is non-zero.
func archiveClosedFeedback(t *testing.T, root string) {
	t.Helper()
	db, err := openLedger(root)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := archive.Run(db, archive.RunOptions{
		SessionHours: 1, Table: "feedback", Yes: true,
	}); err != nil {
		t.Fatal(err)
	}
}

// TestUnionCountsEmptyWhenNoArchive covers a ledger with nothing archived:
// asking how much is archived must answer zero, not error.
func TestUnionCountsEmptyWhenNoArchive(t *testing.T) {
	db, _ := testutil.TempDB(t)
	tc, err := archive.UnionCounts(db)
	if err != nil {
		t.Fatalf("UnionCounts on a fresh ledger: %v", err)
	}
	if tc.Total != 0 {
		t.Fatalf("Total = %d, want 0", tc.Total)
	}
	if tc.BySource == nil {
		t.Fatal("BySource is nil; callers index it without a nil check")
	}
}

// TestLiveCountsMatchesRowCounts proves live=N counts the tables it claims to,
// so it is comparable with archived=M.
func TestLiveCountsMatchesRowCounts(t *testing.T) {
	db, _ := testutil.TempDB(t)
	for i := 0; i < 3; i++ {
		if _, err := db.Exec(`
			INSERT INTO feedback(id, role, severity, category, note, context, status, created_at)
			VALUES (?, 'user', 'low', 'dx', 'x', '', 'open', '2026-01-01T00:00:00Z')`,
			"fb-"+string(rune('a'+i))); err != nil {
			t.Fatal(err)
		}
	}
	lc, err := archive.LiveCounts(db)
	if err != nil {
		t.Fatal(err)
	}
	if got := lc.CountBySource("feedback"); got != 3 {
		t.Fatalf("live feedback = %d, want 3 (BySource=%v)", got, lc.BySource)
	}
	// Every counted table appears, even when empty, so consumers can rely on
	// the key existing.
	for _, table := range archive.CountedTables {
		if _, ok := lc.BySource[table]; !ok {
			t.Fatalf("live counts missing table %q", table)
		}
	}
}

// TestAuditReportsLiveAndArchivedPerProject is acceptance 1: hub audit --json
// gives every project both counts.
func TestAuditReportsLiveAndArchivedPerProject(t *testing.T) {
	root, registry, _ := registerLedger(t, "audited")
	archiveClosedFeedback(t, root)

	report, err := hub.Audit(hub.AuditOptions{Registry: registry})
	if err != nil {
		t.Fatal(err)
	}

	live, ok := report.ByProject["audited"]
	if !ok {
		t.Fatalf("no live counts for the project: %+v", report.ByProject)
	}
	if live["live_feedback"] != 3 {
		t.Fatalf("live_feedback = %d, want 3: %+v", live["live_feedback"], live)
	}
	if live["live_total"] == 0 {
		t.Fatalf("live_total = 0: %+v", live)
	}

	arch, ok := report.ArchivedByProject["audited"]
	if !ok {
		t.Fatalf("no archived counts: %+v", report.ArchivedByProject)
	}
	if arch["total"] != 1 {
		t.Fatalf("archived total = %d, want 1: %+v", arch["total"], arch)
	}
	if arch["feedback"] != 1 {
		t.Fatalf("archived feedback = %d, want 1: %+v", arch["feedback"], arch)
	}

	// Both maps survive JSON with values intact.
	blob, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		ByProject         map[string]map[string]int `json:"by_project"`
		ArchivedByProject map[string]map[string]int `json:"archived_by_project"`
	}
	if err := json.Unmarshal(blob, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.ArchivedByProject["audited"]["total"] != 1 {
		t.Fatalf("archived_by_project lost in JSON: %+v", decoded.ArchivedByProject)
	}
	if decoded.ByProject["audited"]["live_total"] == 0 {
		t.Fatalf("by_project lost live_total in JSON: %+v", decoded.ByProject)
	}
}

// TestAuditArchivedCountsSurviveCachedMode is acceptance 3: cached mode reports
// the same archived and live counts as live mode.
func TestAuditArchivedCountsSurviveCachedMode(t *testing.T) {
	root, registry, metadata := registerLedger(t, "cachedproj")
	archiveClosedFeedback(t, root)

	live, err := hub.Audit(hub.AuditOptions{Registry: registry, MetadataDB: metadata})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hub.SyncAll(registry, metadata, false); err != nil {
		t.Fatalf("sync: %v", err)
	}
	cached, err := hub.Audit(hub.AuditOptions{Registry: registry, MetadataDB: metadata, Mode: "cached"})
	if err != nil {
		t.Fatal(err)
	}

	wantArchived := live.ArchivedByProject["cachedproj"]["total"]
	gotArchived := cached.ArchivedByProject["cachedproj"]["total"]
	if wantArchived == 0 {
		t.Fatal("archived total is 0; the fixture archived nothing")
	}
	if gotArchived != wantArchived {
		t.Fatalf("cached archived total = %d, live = %d", gotArchived, wantArchived)
	}
	wantLive := live.ByProject["cachedproj"]["live_total"]
	gotLive := cached.ByProject["cachedproj"]["live_total"]
	if gotLive != wantLive {
		t.Fatalf("cached live total = %d, live = %d", gotLive, wantLive)
	}
}

// TestAuditArchivedMissingProjectIsNotReported keeps missing ledgers honest:
// they still appear in by_project (so the fleet total is complete) but must not
// claim an archived count they never read.
func TestAuditArchivedMissingProjectIsNotReported(t *testing.T) {
	dir := t.TempDir()
	registry := filepath.Join(dir, "registry")
	// Registry format is one "<path> <alias>" pair per line.
	body := filepath.Join(dir, "nope") + " ghost\n"
	if err := os.WriteFile(registry, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	report, err := hub.Audit(hub.AuditOptions{Registry: registry})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := report.ByProject["ghost"]; !ok {
		t.Fatalf("missing project dropped from by_project: %+v", report.ByProject)
	}
	if _, ok := report.ArchivedByProject["ghost"]; ok {
		t.Fatalf("missing project claims archived counts: %+v", report.ArchivedByProject["ghost"])
	}
}
