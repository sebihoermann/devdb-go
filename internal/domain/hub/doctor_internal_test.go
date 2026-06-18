package hub

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sebihoermann/devdb-go/internal/domain/planning"
	"github.com/sebihoermann/devdb-go/internal/domain/verification"
	"github.com/sebihoermann/devdb-go/internal/storage"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestSourceDBNewerThanBranches(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "development.db")
	if _, err := os.Create(dbPath); err != nil {
		t.Fatal(err)
	}
	if sourceDBNewerThan(filepath.Join(dir, "missing.db"), time.Now().UTC().Format(time.RFC3339Nano)) {
		t.Fatal("missing db should not be newer")
	}
	if !sourceDBNewerThan(dbPath, "not-a-timestamp") {
		t.Fatal("invalid synced timestamp should count as newer")
	}
	future := time.Now().UTC().Add(time.Hour)
	if err := os.Chtimes(dbPath, future, future); err != nil {
		t.Fatal(err)
	}
	past := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
	if !sourceDBNewerThan(dbPath, past) {
		t.Fatal("expected db newer than past sync time")
	}
}

func TestReadSourceSyncError(t *testing.T) {
	db, path := testutil.TempDB(t)
	now := storage.NowUTC()
	_, err := db.Exec(`INSERT INTO sync_state(key, value, updated_at) VALUES ('last_metadata_push_error', 'boom', ?)`, now)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	if got := readSourceSyncError(path); got != "boom" {
		t.Fatalf("sync error=%q", got)
	}
	if got := readSourceSyncError(filepath.Join(filepath.Dir(path), "missing.db")); got != "" {
		t.Fatalf("missing=%q", got)
	}
}

func TestDiagnoseFreshnessUnknown(t *testing.T) {
	p := RegisteredProject{Alias: "x", Exists: true, DBPath: filepath.Join(t.TempDir(), "nope.db")}
	row := DoctorRow{HubSyncedAt: ""}
	status, cmd := diagnoseFreshness(p, row)
	if status != "stale" || cmd == "" {
		t.Fatalf("stale unsynced: status=%q cmd=%q", status, cmd)
	}
}

func TestAttentionScoringHelpers(t *testing.T) {
	missing := Snapshot{SyncStatus: "missing", AttentionScore: 100}
	if attentionScore(missing) != 100 {
		t.Fatal("missing should preserve preset score")
	}
	active := Snapshot{
		SyncStatus: "active", OpenHighFeedback: 1, OpenHighFindings: 1,
		StaleArchNotes: 2, LatestVerificationStatus: "passed",
		LatestVerificationFreshness: "stale", GitDirty: true, BlockedReason: "blocked",
		MissedCalls7d: 10, OverdueReminders: 1,
	}
	score := attentionScore(active)
	if score < 80 {
		t.Fatalf("score=%d", score)
	}
	items := buildAttention(active)
	if len(items) < 6 {
		t.Fatalf("items=%+v", items)
	}
	if len(buildAttention(Snapshot{Error: "boom", AttentionItems: []AttentionItem{{Kind: "x"}}})) != 1 {
		t.Fatal("error snapshot should keep preset attention items")
	}
}

func TestCollectSnapshotMissingAndUnreadable(t *testing.T) {
	dir := t.TempDir()
	missing := CollectSnapshot(dir, filepath.Join(dir, ".devdb", "development.db"))
	if missing.SyncStatus != "missing" || missing.AttentionScore != 100 {
		t.Fatalf("missing=%+v", missing)
	}
	badPath := filepath.Join(dir, "bad.db")
	if err := os.WriteFile(badPath, []byte("not-db"), 0o644); err != nil {
		t.Fatal(err)
	}
	bad := CollectSnapshot(dir, badPath)
	if bad.SyncStatus != "error" {
		t.Fatalf("bad=%+v", bad)
	}
}

func TestEncodeDecodeSnapshot(t *testing.T) {
	raw, err := encodeSnapshot(Snapshot{WorkStatus: "idle"})
	if err != nil || raw == "" {
		t.Fatalf("encode: %q err=%v", raw, err)
	}
	if _, err := decodeSnapshot(""); err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeSnapshot(raw)
	if err != nil || decoded.WorkStatus != "idle" {
		t.Fatalf("decode=%+v err=%v", decoded, err)
	}
}

func TestLatestVerificationAndBlockedReason(t *testing.T) {
	db, _ := testutil.TempDB(t)
	planID, _ := planning.CreatePlan(db, planning.CreatePlanInput{Slug: "p", Title: "P", ModelID: "test"})
	itemID, _ := planning.AddItem(db, planning.AddItemInput{PlanID: planID, Title: "blocked", ModelID: "test"})
	_, _ = planning.StartItem(db, itemID, "test")
	_, _ = planning.PauseItem(db, itemID, "blocked: waiting on review", "test")

	exit := 0
	runID, _ := verification.RecordRun(db, "go test ./...", ".", "", "passed", &exit, "", "", "test")
	_ = verification.FinishRun(db, runID, "passed", &exit, "")

	status, freshness, command, scope, finishedAt := latestVerification(db)
	if status != "passed" || command == "" || scope == "" || finishedAt == "" {
		t.Fatalf("verification=%q %q %q %q %q", status, freshness, command, scope, finishedAt)
	}
	if reason := blockedReason(db); reason == "" {
		t.Fatal("expected blocked reason")
	}
}

func TestCollectSnapshotFullProject(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := filepath.Join(home, "proj")
	testutil.InitGitRepo(t, root)
	db, path := testutil.TempDB(t)
	_, _ = db.Exec(`INSERT INTO review_runs(id, scope_paths, tier, started_at, model_id) VALUES ('r1','.','default',datetime('now'),'test')`)
	_, _ = db.Exec(`INSERT INTO review_findings(
		id, run_id, principle, title, recommendation, severity, confidence, effort, status, created_at, model_id
	) VALUES ('f1','r1','dry','issue','fix','high','high','small','open',datetime('now'),'test')`)
	exit := 0
	runID, _ := verification.RecordRun(db, "go test ./...", ".", "", "passed", &exit, "", "", "test")
	_ = verification.FinishRun(db, runID, "passed", &exit, "")
	db.Close()
	devdbDir := filepath.Join(root, ".devdb")
	if err := os.MkdirAll(devdbDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, filepath.Join(devdbDir, "development.db")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "dirty.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	snap := CollectSnapshot("~/proj", filepath.Join(devdbDir, "development.db"))
	if snap.SyncStatus != "active" || snap.OpenHighFindings < 1 {
		t.Fatalf("snapshot=%+v", snap)
	}
	if snap.GitDirty && snap.AttentionScore < 10 {
		t.Fatalf("expected attention for dirty tree: %+v", snap)
	}
}

func TestHubHelperEdgeCases(t *testing.T) {
	markSourceSynced("", "now")
	MarkDirty("")
	if readSourceDirty(filepath.Join(t.TempDir(), "missing.db")) {
		t.Fatal("missing db should not be dirty")
	}
	if readLastHubSync(filepath.Join(t.TempDir(), "missing.db")) != "" {
		t.Fatal("expected empty last hub sync")
	}
	if !verificationNeedsAttention("stale") {
		t.Fatal("stale should need attention")
	}
	if verificationNeedsAttention("fresh") || verificationNeedsAttention("") {
		t.Fatal("fresh/empty should not need attention")
	}
}

func TestCollectSnapshotLegacyPython(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "legacy.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE schema_migrations (version INTEGER, description TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO schema_migrations(version, description) VALUES (1, 'python')`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	snap := CollectSnapshot(dir, dbPath)
	if snap.SyncStatus != "error" || snap.AttentionScore != 80 {
		t.Fatalf("snap=%+v", snap)
	}
}

func TestCollectSnapshotRichAttention(t *testing.T) {
	db, path := testutil.TempDB(t)
	for i := 0; i < 10; i++ {
		id, _ := storage.NewID()
		_, _ = db.Exec(`INSERT INTO missed_cli_calls(id, raw_argv, failure_kind, error_message, exit_code, cwd, repo_root, model_id, created_at)
			VALUES (?, '[]', 'unknown_command', 'err', 2, '/', '/', 'test', datetime('now'))`, id)
	}
	_, _ = db.Exec(`INSERT INTO feedback(id, role, severity, note, created_at) VALUES ('hf','codebase','critical','bad',datetime('now'))`)
	archID, _ := storage.NewID()
	_, _ = db.Exec(`INSERT INTO architecture_notes
		(id, topic, body, source_paths, source_hashes, confidence, status, last_verified_at, created_at, updated_at, model_id)
		VALUES (?, 'stale', 'body', '[]', '{}', 'medium', 'stale', datetime('now','-30 days'), datetime('now'), datetime('now'), 'test')`, archID)
	_, _ = db.Exec(`INSERT INTO reminders(id, title, status, due_at, created_at, model_id)
		VALUES ('rem','due','open',datetime('now','-1 day'),datetime('now'),'test')`)
	_, _ = db.Exec(`INSERT INTO scan_runs(id, started_at, finished_at, files_seen, files_changed, model_id)
		VALUES ('scan1', datetime('now'), datetime('now'), 12, 3, 'test')`)
	planID, _ := planning.CreatePlan(db, planning.CreatePlanInput{Slug: "p", Title: "P", ModelID: "test"})
	itemID, _ := planning.AddItem(db, planning.AddItemInput{PlanID: planID, Title: "work", ModelID: "test"})
	_, _ = planning.StartItem(db, itemID, "test")
	exit := 1
	runID, _ := verification.RecordRun(db, "go test ./...", ".", "", "failed", &exit, "", "", "test")
	_ = verification.FinishRun(db, runID, "failed", &exit, "")
	db.Close()

	root := filepath.Dir(filepath.Dir(path))
	snap := CollectSnapshot(root, path)
	if snap.AttentionScore < 20 || len(snap.AttentionItems) < 3 {
		t.Fatalf("snapshot=%+v", snap)
	}
	if snap.FilesIndexed != 12 || snap.FilesChanged != 3 {
		t.Fatalf("scan stats=%d/%d", snap.FilesIndexed, snap.FilesChanged)
	}
	_ = archID
}

func TestDecodeSnapshotInvalidJSON(t *testing.T) {
	if _, err := decodeSnapshot("{"); err == nil {
		t.Fatal("expected decode error")
	}
}
