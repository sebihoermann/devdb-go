package hub_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sebihoermann/devdb-go/internal/domain/hub"
	"github.com/sebihoermann/devdb-go/internal/domain/planning"
	"github.com/sebihoermann/devdb-go/internal/domain/verification"
	"github.com/sebihoermann/devdb-go/internal/migrate"
	"github.com/sebihoermann/devdb-go/internal/storage"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestRegisterSyncDashboardDoctor(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	registry := filepath.Join(dir, "registry")
	metaDB := filepath.Join(dir, "metadata.db")

	root := filepath.Join(dir, "proj-a")
	if err := os.MkdirAll(filepath.Join(root, ".devdb"), 0o755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(root, ".devdb", "development.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrate.RunAll(db); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO feedback(id, role, note, created_at) VALUES ('f1','codebase','hub test',datetime('now'))`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	p, err := hub.Register(root, "proj-a", registry, metaDB)
	if err != nil {
		t.Fatal(err)
	}
	if p.Alias != "proj-a" {
		t.Fatalf("alias=%q", p.Alias)
	}

	res, err := hub.SyncAll(registry, metaDB, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.ProjectsSeen != 1 || res.ProjectsUpdated != 1 {
		t.Fatalf("sync: %+v", res)
	}

	rows, err := hub.Dashboard(registry, metaDB, "summary", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Alias != "proj-a" {
		t.Fatalf("dashboard: %+v", rows)
	}

	detail, err := hub.Project(registry, metaDB, "proj-a")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Snapshot.OpenFeedback < 1 {
		t.Fatalf("expected feedback in snapshot: %+v", detail.Snapshot)
	}

	doctor, err := hub.Doctor(registry, metaDB, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(doctor) != 1 {
		t.Fatalf("doctor rows: %+v", doctor)
	}
	if doctor[0].FreshnessStatus != "fresh" {
		t.Fatalf("expected fresh, got %q", doctor[0].FreshnessStatus)
	}
}

func TestDirtyRepairAndMissingProject(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	registry := filepath.Join(dir, "registry")
	metaDB := filepath.Join(dir, "metadata.db")

	root := filepath.Join(dir, "proj-b")
	db, path := testutil.TempDB(t)
	db.Close()
	devdbDir := filepath.Join(root, ".devdb")
	if err := os.MkdirAll(devdbDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, filepath.Join(devdbDir, "development.db")); err != nil {
		t.Fatal(err)
	}

	if _, err := hub.Register(root, "proj-b", registry, metaDB); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.SyncAll(registry, metaDB, false); err != nil {
		t.Fatal(err)
	}

	hub.MarkDirty(filepath.Join(devdbDir, "development.db"))
	doctor, err := hub.Doctor(registry, metaDB, "proj-b")
	if err != nil {
		t.Fatal(err)
	}
	if len(doctor) != 1 || doctor[0].FreshnessStatus != "dirty" {
		t.Fatalf("expected dirty: %+v", doctor)
	}

	entries, err := hub.List(registry, metaDB, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("list: %+v", entries)
	}

	missingRoot := filepath.Join(dir, "missing")
	if _, err := hub.Register(missingRoot, "missing", registry, metaDB); err != nil {
		t.Fatal(err)
	}
	res, err := hub.SyncAll(registry, metaDB, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.ProjectsFailed < 1 {
		t.Fatalf("expected missing project failure: %+v", res)
	}
	attn, err := hub.Dashboard(registry, metaDB, "summary", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(attn) < 1 {
		t.Fatal("expected attention-only row for missing project")
	}
}

func TestAcrossOpenDebt(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	registry := filepath.Join(dir, "registry")
	metaDB := filepath.Join(dir, "metadata.db")

	root := filepath.Join(dir, "proj-c")
	db, path := testutil.TempDB(t)
	_, err := db.Exec(`INSERT INTO review_runs(id, scope_paths, tier, started_at, model_id) VALUES ('run1','src','default',datetime('now'),'unknown')`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO review_findings(
		id, run_id, principle, title, recommendation, severity, confidence, effort, status, created_at, model_id
	) VALUES ('rf1','run1','correctness','bad thing','fix it','high','high','low','open',datetime('now'),'unknown')`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	devdbDir := filepath.Join(root, ".devdb")
	if err := os.MkdirAll(devdbDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, filepath.Join(devdbDir, "development.db")); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.Register(root, "proj-c", registry, metaDB); err != nil {
		t.Fatal(err)
	}

	rows, err := hub.Across(hub.AcrossOptions{Query: "open-debt", Registry: registry})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["project"] != "proj-c" {
		t.Fatalf("across: %+v", rows)
	}
}

func TestAttentionRanking(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	registry := filepath.Join(dir, "registry")
	metaDB := filepath.Join(dir, "metadata.db")

	makeProject := func(name string, note string) {
		root := filepath.Join(dir, name)
		db, path := testutil.TempDB(t)
		if note != "" {
			_, err := db.Exec(`INSERT INTO feedback(id, role, severity, note, created_at)
				VALUES (?, 'codebase', 'critical', ?, datetime('now'))`, name+"f", note)
			if err != nil {
				t.Fatal(err)
			}
		}
		db.Close()
		devdbDir := filepath.Join(root, ".devdb")
		if err := os.MkdirAll(devdbDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(path, filepath.Join(devdbDir, "development.db")); err != nil {
			t.Fatal(err)
		}
		if _, err := hub.Register(root, name, registry, metaDB); err != nil {
			t.Fatal(err)
		}
	}
	makeProject("quiet", "")
	makeProject("noisy", "critical issue")

	if _, err := hub.SyncAll(registry, metaDB, false); err != nil {
		t.Fatal(err)
	}
	rows, err := hub.Dashboard(registry, metaDB, "quality", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) < 2 {
		t.Fatalf("rows: %+v", rows)
	}
	if rows[0].Alias != "noisy" {
		t.Fatalf("expected noisy first, got %+v", rows)
	}
}

func TestDoctorFreshnessBranches(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	registry := filepath.Join(dir, "registry")
	metaDB := filepath.Join(dir, "metadata.db")

	root := filepath.Join(dir, "fresh")
	db, path := testutil.TempDB(t)
	db.Close()
	devdbDir := filepath.Join(root, ".devdb")
	if err := os.MkdirAll(devdbDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(devdbDir, "development.db")
	if err := os.Rename(path, dbPath); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.Register(root, "fresh", registry, metaDB); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.SyncAll(registry, metaDB, false); err != nil {
		t.Fatal(err)
	}

	// Unsynced marker: clear last_hub_sync and touch DB newer than hub snapshot.
	srcDB, err := storage.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = srcDB.Exec(`DELETE FROM sync_state WHERE key='last_hub_sync_at'`)
	srcDB.Close()
	future := time.Now().UTC().Add(2 * time.Hour)
	if err := os.Chtimes(dbPath, future, future); err != nil {
		t.Fatal(err)
	}
	doctor, err := hub.Doctor(registry, metaDB, "fresh")
	if err != nil {
		t.Fatal(err)
	}
	if len(doctor) != 1 || doctor[0].FreshnessStatus != "dirty" {
		t.Fatalf("expected dirty after newer mtime: %+v", doctor)
	}

	// Stale when never synced to hub row timestamps.
	rootStale := filepath.Join(dir, "stale")
	if err := os.MkdirAll(filepath.Join(rootStale, ".devdb"), 0o755); err != nil {
		t.Fatal(err)
	}
	staleDB, stalePath := testutil.TempDB(t)
	staleDB.Close()
	if err := os.Rename(stalePath, filepath.Join(rootStale, ".devdb", "development.db")); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.Register(rootStale, "stale", registry, metaDB); err != nil {
		t.Fatal(err)
	}
	hubDB, err := hub.OpenHub(metaDB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hubDB.Exec(`DELETE FROM project_snapshots WHERE alias='stale'`); err != nil {
		t.Fatal(err)
	}
	hubDB.Close()
	doctor, err = hub.Doctor(registry, metaDB, "stale")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range doctor {
		if row.Alias == "stale" && row.FreshnessStatus != "stale" {
			t.Fatalf("expected stale freshness: %+v", row)
		}
	}
}

func TestDoctorSyncErrorFromSource(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	registry := filepath.Join(dir, "registry")
	metaDB := filepath.Join(dir, "metadata.db")
	root := filepath.Join(dir, "errproj")
	db, path := testutil.TempDB(t)
	_, _ = db.Exec(`
		INSERT INTO sync_state(key, value, updated_at) VALUES
		('last_metadata_push_error', 'push failed', datetime('now')),
		('last_hub_sync_at', datetime('now'), datetime('now'))`)
	db.Close()
	devdbDir := filepath.Join(root, ".devdb")
	if err := os.MkdirAll(devdbDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, filepath.Join(devdbDir, "development.db")); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.Register(root, "errproj", registry, metaDB); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.SyncAll(registry, metaDB, false); err != nil {
		t.Fatal(err)
	}
	// Inject snapshot error so doctor reports error freshness.
	hubDB, err := hub.OpenHub(metaDB)
	if err != nil {
		t.Fatal(err)
	}
	_, err = hubDB.Exec(`UPDATE project_snapshots SET snapshot_json=json_set(snapshot_json,'$.error','sync blew up') WHERE alias='errproj'`)
	if err != nil {
		t.Fatal(err)
	}
	hubDB.Close()

	doctor, err := hub.Doctor(registry, metaDB, "errproj")
	if err != nil {
		t.Fatal(err)
	}
	if len(doctor) != 1 || doctor[0].FreshnessStatus != "error" {
		t.Fatalf("doctor=%+v", doctor)
	}
	if doctor[0].LastSyncError == "" {
		t.Fatal("expected last sync error populated")
	}
}

func TestSyncAllStrictAndProjectByPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	registry := filepath.Join(dir, "registry")
	metaDB := filepath.Join(dir, "metadata.db")
	root := filepath.Join(dir, "by-path")
	db, path := testutil.TempDB(t)
	db.Close()
	if err := os.MkdirAll(filepath.Join(root, ".devdb"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, filepath.Join(root, ".devdb", "development.db")); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.Register(root, "by-path", registry, metaDB); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.SyncAll(registry, metaDB, false); err != nil {
		t.Fatal(err)
	}
	detail, err := hub.Project(registry, metaDB, root)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Alias != "by-path" {
		t.Fatalf("detail=%+v", detail)
	}
	if _, err := hub.Project(registry, metaDB, "missing-alias"); err == nil {
		t.Fatal("expected not found")
	}

	missingRoot := filepath.Join(dir, "strict-miss")
	if _, err := hub.Register(missingRoot, "strict-miss", registry, metaDB); err != nil {
		t.Fatal(err)
	}
	res, err := hub.SyncAll(registry, metaDB, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "failed" || res.ProjectsFailed < 1 {
		t.Fatalf("strict sync=%+v", res)
	}
}

func TestSnapshotAttentionScoring(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	registry := filepath.Join(dir, "registry")
	metaDB := filepath.Join(dir, "metadata.db")
	root := filepath.Join(dir, "attention")
	db, path := testutil.TempDB(t)

	for i := 0; i < 10; i++ {
		id, _ := storage.NewID()
		_, _ = db.Exec(`INSERT INTO missed_cli_calls(id, raw_argv, failure_kind, error_message, exit_code, cwd, repo_root, model_id, created_at)
			VALUES (?, '[]', 'unknown_command', 'err', 2, '/', '/', 'test', datetime('now'))`, id)
	}
	_, _ = db.Exec(`INSERT INTO feedback(id, role, severity, note, created_at) VALUES ('hf','codebase','critical','bad',datetime('now'))`)
	id, _ := storage.NewID()
	_, _ = db.Exec(`INSERT INTO architecture_notes
		(id, topic, body, source_paths, source_hashes, confidence, status, last_verified_at, created_at, updated_at, model_id)
		VALUES (?, 'stale', 'body', '[]', '{}', 'medium', 'stale', datetime('now','-30 days'), datetime('now'), datetime('now'), 'test')`, id)
	_, _ = db.Exec(`INSERT INTO reminders(id, title, status, due_at, created_at, model_id)
		VALUES ('rem','due soon','open',datetime('now','-1 day'),datetime('now'),'test')`)
	planID, _ := planning.CreatePlan(db, planning.CreatePlanInput{Slug: "p", Title: "P", ModelID: "test"})
	itemID, _ := planning.AddItem(db, planning.AddItemInput{PlanID: planID, Title: "active work", ModelID: "test"})
	_, _ = planning.StartItem(db, itemID, "test")

	db.Close()
	if err := os.MkdirAll(filepath.Join(root, ".devdb"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, filepath.Join(root, ".devdb", "development.db")); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.Register(root, "attention", registry, metaDB); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.SyncAll(registry, metaDB, false); err != nil {
		t.Fatal(err)
	}
	detail, err := hub.Project(registry, metaDB, "attention")
	if err != nil {
		t.Fatal(err)
	}
	s := detail.Snapshot
	if s.AttentionScore < 25 {
		t.Fatalf("score=%d snapshot=%+v", s.AttentionScore, s)
	}
	kinds := map[string]bool{}
	for _, a := range detail.Attention {
		kinds[a.Kind] = true
	}
	for _, want := range []string{"high_feedback", "stale_arch", "missed_cli", "overdue_reminder", "in_progress"} {
		if !kinds[want] {
			t.Fatalf("missing attention kind %q in %+v", want, detail.Attention)
		}
	}
}

func TestCollectSnapshotPythonSchemaError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	registry := filepath.Join(dir, "registry")
	metaDB := filepath.Join(dir, "metadata.db")
	root := filepath.Join(dir, "legacy")
	if err := os.MkdirAll(filepath.Join(root, ".devdb"), 0o755); err != nil {
		t.Fatal(err)
	}
	legacyPath := filepath.Join(root, ".devdb", "development.db")
	legacyDB, err := storage.Open(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacyDB.Exec(`CREATE TABLE schema_migrations (version INTEGER, description TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := legacyDB.Exec(`INSERT INTO schema_migrations(version, description) VALUES (1, 'python')`); err != nil {
		t.Fatal(err)
	}
	legacyDB.Close()

	if _, err := hub.Register(root, "legacy", registry, metaDB); err != nil {
		t.Fatal(err)
	}
	res, err := hub.SyncAll(registry, metaDB, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.ProjectsFailed < 1 {
		t.Fatalf("expected legacy sync failure: %+v", res)
	}
	detail, err := hub.Project(registry, metaDB, "legacy")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Snapshot.SyncStatus != "error" {
		t.Fatalf("snapshot=%+v", detail.Snapshot)
	}
}

func TestListRefreshesWhenSourceDBNewer(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	registry := filepath.Join(dir, "registry")
	metaDB := filepath.Join(dir, "metadata.db")
	root := filepath.Join(dir, "refresh")
	db, path := testutil.TempDB(t)
	_, _ = db.Exec(`INSERT INTO feedback(id, role, note, created_at) VALUES ('f1','user','before sync',datetime('now'))`)
	db.Close()
	if err := os.MkdirAll(filepath.Join(root, ".devdb"), 0o755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(root, ".devdb", "development.db")
	if err := os.Rename(path, dbPath); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.Register(root, "refresh", registry, metaDB); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.SyncAll(registry, metaDB, false); err != nil {
		t.Fatal(err)
	}

	srcDB, err := storage.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = srcDB.Exec(`DELETE FROM sync_state WHERE key='last_hub_sync_at'`)
	_, _ = srcDB.Exec(`INSERT INTO feedback(id, role, note, created_at) VALUES ('f2','user','after sync',datetime('now'))`)
	srcDB.Close()
	future := time.Now().UTC().Add(3 * time.Hour)
	if err := os.Chtimes(dbPath, future, future); err != nil {
		t.Fatal(err)
	}

	entries, err := hub.List(registry, metaDB, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Snapshot == nil || entries[0].Snapshot.OpenFeedback < 2 {
		t.Fatalf("expected refreshed snapshot with 2 feedback rows: %+v", entries[0])
	}
}

func TestDashboardAttentionOnlyVerificationStale(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	registry := filepath.Join(dir, "registry")
	metaDB := filepath.Join(dir, "metadata.db")
	root := filepath.Join(dir, "verify")
	testutil.InitGitRepo(t, root)
	db, path := testutil.TempDB(t)
	exit := 0
	runID, _ := verification.RecordRun(db, "go test ./...", ".", "", "passed", &exit, "", "", "test")
	_ = verification.FinishRun(db, runID, "passed", &exit, "")
	db.Close()
	if err := os.MkdirAll(filepath.Join(root, ".devdb"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, filepath.Join(root, ".devdb", "development.db")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "dirty.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.Register(root, "verify", registry, metaDB); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.SyncAll(registry, metaDB, false); err != nil {
		t.Fatal(err)
	}
	rows, err := hub.Dashboard(registry, metaDB, "quality", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) < 1 {
		t.Fatal("expected attention-only row for dirty/stale verification")
	}
	found := false
	for _, r := range rows {
		if r.Alias == "verify" {
			found = true
		}
	}
	if !found {
		t.Fatalf("rows=%+v", rows)
	}
}

func TestDoctorFreshPathAndSourceSyncError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	registry := filepath.Join(dir, "registry")
	metaDB := filepath.Join(dir, "metadata.db")
	root := filepath.Join(dir, "syncerr")
	db, path := testutil.TempDB(t)
	now := storage.NowUTC()
	_, _ = db.Exec(`INSERT INTO sync_state(key, value, updated_at) VALUES ('last_metadata_push_error', 'metadata push failed', ?)`, now)
	_, _ = db.Exec(`INSERT INTO sync_state(key, value, updated_at) VALUES ('last_hub_sync_at', ?, ?)`, now, now)
	db.Close()
	if err := os.MkdirAll(filepath.Join(root, ".devdb"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, filepath.Join(root, ".devdb", "development.db")); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.Register(root, "syncerr", registry, metaDB); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.SyncAll(registry, metaDB, false); err != nil {
		t.Fatal(err)
	}
	doctor, err := hub.Doctor(registry, metaDB, "syncerr")
	if err != nil {
		t.Fatal(err)
	}
	if len(doctor) != 1 {
		t.Fatalf("doctor=%+v", doctor)
	}
	if doctor[0].FreshnessStatus != "fresh" {
		t.Fatalf("expected fresh with matching hub sync: %+v", doctor[0])
	}

	hubDB, err := hub.OpenHub(metaDB)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = hubDB.Exec(`UPDATE project_snapshots SET snapshot_json=json_set(snapshot_json,'$.error','') WHERE alias='syncerr'`)
	_, _ = hubDB.Exec(`UPDATE project_snapshots SET synced_at='not-a-valid-time' WHERE alias='syncerr'`)
	hubDB.Close()
	doctor, err = hub.Doctor(registry, metaDB, "syncerr")
	if err != nil {
		t.Fatal(err)
	}
	if len(doctor) != 1 {
		t.Fatal("expected doctor row")
	}
}

func TestDoctorFilterByRootPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	registry := filepath.Join(dir, "registry")
	metaDB := filepath.Join(dir, "metadata.db")
	root := filepath.Join(dir, "filter")
	db, path := testutil.TempDB(t)
	db.Close()
	if err := os.MkdirAll(filepath.Join(root, ".devdb"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, filepath.Join(root, ".devdb", "development.db")); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.Register(root, "filter", registry, metaDB); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.SyncAll(registry, metaDB, false); err != nil {
		t.Fatal(err)
	}
	doctor, err := hub.Doctor(registry, metaDB, root)
	if err != nil || len(doctor) != 1 || doctor[0].Alias != "filter" {
		t.Fatalf("doctor=%+v err=%v", doctor, err)
	}
}

func TestOpenHubRunHubFailureOnReadonlyFile(t *testing.T) {
	dir := t.TempDir()
	meta := filepath.Join(dir, "meta.db")
	if err := os.WriteFile(meta, []byte{0}, 0o444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(meta, 0o666) })
	if _, err := hub.OpenHub(meta); err == nil {
		t.Fatal("expected open hub failure on readonly metadata file")
	}
}

func TestOpenHubInvalidMetadataPath(t *testing.T) {
	dir := t.TempDir()
	badMeta := filepath.Join(dir, "meta")
	if err := os.MkdirAll(badMeta, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.OpenHub(badMeta); err == nil {
		t.Fatal("expected error when metadata path is a directory")
	}
}

func TestListWithoutRefreshLeavesMissingSnapshot(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	registry := filepath.Join(dir, "registry")
	metaDB := filepath.Join(dir, "metadata.db")
	root := filepath.Join(dir, "nosnap")
	if err := os.MkdirAll(filepath.Join(root, ".devdb"), 0o755); err != nil {
		t.Fatal(err)
	}
	db, path := testutil.TempDB(t)
	db.Close()
	if err := os.Rename(path, filepath.Join(root, ".devdb", "development.db")); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.Register(root, "nosnap", registry, metaDB); err != nil {
		t.Fatal(err)
	}
	hubDB, err := hub.OpenHub(metaDB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hubDB.Exec(`DELETE FROM project_snapshots WHERE alias='nosnap'`); err != nil {
		t.Fatal(err)
	}
	hubDB.Close()
	entries, err := hub.List(registry, metaDB, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Snapshot != nil {
		t.Fatalf("entries=%+v", entries)
	}
}

func TestRegisterDefaultAliasFromPath(t *testing.T) {
	dir := t.TempDir()
	registry := filepath.Join(dir, "registry")
	metaDB := filepath.Join(dir, "metadata.db")
	root := filepath.Join(dir, "my project")
	if err := os.MkdirAll(filepath.Join(root, ".devdb"), 0o755); err != nil {
		t.Fatal(err)
	}
	db, path := testutil.TempDB(t)
	db.Close()
	if err := os.Rename(path, filepath.Join(root, ".devdb", "development.db")); err != nil {
		t.Fatal(err)
	}
	p, err := hub.Register(root, "", registry, metaDB)
	if err != nil {
		t.Fatal(err)
	}
	if p.Alias != "my-project" {
		t.Fatalf("alias=%q", p.Alias)
	}
}

func TestOpenHubDefaultMetadataPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	metaDB, err := hub.OpenHub("")
	if err != nil {
		t.Fatal(err)
	}
	if err := metaDB.Close(); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".devdb", "metadata.db")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("metadata db not created at %s: %v", want, err)
	}
}
