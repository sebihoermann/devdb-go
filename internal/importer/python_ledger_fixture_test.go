package importer_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/importer"
	"github.com/sebihoermann/devdb-go/internal/migrate"
	"github.com/sebihoermann/devdb-go/internal/storage"
)

func createMinimalPythonDB(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	stmts := []string{
		`CREATE TABLE schema_migrations (version INTEGER, description TEXT)`,
		`INSERT INTO schema_migrations(version, description) VALUES (1, 'python init')`,
		`CREATE TABLE goals (
			id TEXT PRIMARY KEY, kind TEXT NOT NULL, title TEXT NOT NULL, body TEXT,
			status TEXT NOT NULL, created_at TEXT NOT NULL, model_id TEXT NOT NULL)`,
		`CREATE TABLE feedback (
			id TEXT PRIMARY KEY, role TEXT NOT NULL, category TEXT, severity TEXT,
			note TEXT NOT NULL, context TEXT, status TEXT NOT NULL, proposed_fix TEXT,
			created_at TEXT NOT NULL, model_id TEXT NOT NULL)`,
		`CREATE TABLE plans (
			id TEXT PRIMARY KEY, slug TEXT NOT NULL, title TEXT NOT NULL, body TEXT,
			status TEXT NOT NULL, created_at TEXT NOT NULL, model_id TEXT NOT NULL)`,
		`CREATE TABLE plan_items (
			id TEXT PRIMARY KEY, plan_id TEXT, milestone_id TEXT, item_number INTEGER,
			phase TEXT, step TEXT, title TEXT NOT NULL, body TEXT, source_doc TEXT,
			status TEXT NOT NULL, approval_status TEXT NOT NULL DEFAULT 'none',
			created_at TEXT NOT NULL, model_id TEXT NOT NULL)`,
		`CREATE TABLE repo_files (
			path TEXT PRIMARY KEY, language TEXT, kind TEXT NOT NULL, lines INTEGER,
			content_hash TEXT, size_bytes INTEGER, last_seen_at TEXT NOT NULL, last_scan_run_id TEXT)`,
		`CREATE TABLE verification_runs (
			id TEXT PRIMARY KEY, command TEXT, scope TEXT, status TEXT, git_sha TEXT,
			exit_code INTEGER, output TEXT, notes TEXT, started_at TEXT, finished_at TEXT,
			dismissed_at TEXT, dismissed_reason TEXT, created_at TEXT NOT NULL, model_id TEXT NOT NULL)`,
		`CREATE TABLE archive_entries (
			id TEXT PRIMARY KEY, source_table TEXT NOT NULL, source_id TEXT NOT NULL,
			payload_json TEXT NOT NULL, archived_at TEXT NOT NULL, archive_reason TEXT)`,
		`INSERT INTO goals(id, kind, title, status, created_at, model_id)
			VALUES ('g1', 'goal', 'Ship', 'inactive', '2020-01-01T00:00:00Z', 'test')`,
		`INSERT INTO feedback(id, role, note, status, created_at, model_id)
			VALUES ('fb1', 'model', 'legacy note', 'deferred', '2020-01-01T00:00:00Z', 'test')`,
		`INSERT INTO feedback(id, role, note, status, created_at, model_id)
			VALUES ('fb2', 'model', 'wontfix note', 'wontfix', '2020-01-01T00:00:00Z', 'test')`,
		`INSERT INTO feedback(id, role, note, status, created_at, model_id)
			VALUES ('fb3', 'model', 'resolved note', 'resolved', '2020-01-01T00:00:00Z', 'test')`,
		`INSERT INTO plans(id, slug, title, status, created_at, model_id)
			VALUES ('p1', 'plan-a', 'Plan A', 'active', '2020-01-01T00:00:00Z', 'test')`,
		`INSERT INTO plan_items(id, plan_id, title, status, created_at, model_id)
			VALUES ('pi1', 'p1', 'Item', 'done', '2020-01-01T00:00:00Z', 'test')`,
		`INSERT INTO repo_files(path, kind, last_seen_at)
			VALUES ('main.go', 'code', '2020-01-01T00:00:00Z')`,
		`INSERT INTO verification_runs(id, command, scope, status, git_sha, started_at, created_at, model_id)
			VALUES ('vr1', 'go test ./...', '.', 'passed', 'abc', '2020-01-01T00:00:00Z', '2020-01-01T00:00:00Z', 'test')`,
		`INSERT INTO archive_entries(id, source_table, source_id, payload_json, archived_at)
			VALUES ('ae1', 'features', 'f1', '{"id":"f1"}', '2020-01-01T00:00:00Z')`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("setup %q: %v", stmt, err)
		}
	}
	return path
}

func TestImportPythonDBFromFixture(t *testing.T) {
	src := createMinimalPythonDB(t, t.TempDir(), "legacy.db")
	dst := filepath.Join(t.TempDir(), "development.db")

	result, err := importer.ImportPythonDB(src, dst, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Tables["feedback"] != 3 || result.Tables["goals"] != 1 || result.Tables["plan_items"] != 1 {
		t.Fatalf("copy counts: %+v", result.Tables)
	}
	if len(result.Skipped) == 0 {
		t.Fatal("expected skipped python-only tables")
	}

	dstDB, err := storage.Open(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer dstDB.Close()

	kind, _, err := storage.DetectSchema(dstDB)
	if err != nil || kind != storage.SchemaGo {
		t.Fatalf("schema=%s err=%v", kind, err)
	}
	var status string
	if err := dstDB.QueryRow(`SELECT status FROM goals WHERE id='g1'`).Scan(&status); err != nil || status != "wontfix" {
		t.Fatalf("goal status mapped: %q err=%v", status, err)
	}
	if err := dstDB.QueryRow(`SELECT status FROM feedback WHERE id='fb1'`).Scan(&status); err != nil || status != "closed" {
		t.Fatalf("feedback status mapped: %q err=%v", status, err)
	}
}

func TestApplyInPlace(t *testing.T) {
	dir := t.TempDir()
	srcPath := createMinimalPythonDB(t, dir, "development.db")

	result, err := importer.ApplyInPlace(srcPath, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.DestPath != srcPath {
		t.Fatalf("dest path: %s", result.DestPath)
	}
	backup := filepath.Join(dir, "development.db.python-bak")
	if _, err := os.Stat(backup); err != nil {
		t.Fatalf("backup missing: %v", err)
	}

	db, err := storage.Open(srcPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	kind, _, err := storage.DetectSchema(db)
	if err != nil || kind != storage.SchemaGo {
		t.Fatalf("in-place schema=%s err=%v", kind, err)
	}
	got, err := importer.CountParity(db)
	if err != nil {
		t.Fatal(err)
	}
	if got.Feedback != 3 || got.Goals != 1 || got.PlanItems != 1 || got.Plans != 1 {
		t.Fatalf("parity after apply: %+v", got)
	}
}

func TestCountParityPartialSchema(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "partial.db")
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE feedback (id TEXT PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO feedback(id) VALUES ('x')`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	db, err = storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	got, err := importer.CountParity(db)
	if err != nil {
		t.Fatal(err)
	}
	if got.Feedback != 1 || got.PlanItems != 0 {
		t.Fatalf("partial parity: %+v", got)
	}
}

func TestInspectPythonDBRejectsGoSchema(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "go.db")
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrate.RunAll(db); err != nil {
		t.Fatal(err)
	}
	db.Close()

	_, err = importer.InspectPythonDB(path)
	if err == nil {
		t.Fatal("expected reject for go schema")
	}
}

func TestImportRejectsPythonDestination(t *testing.T) {
	dir := t.TempDir()
	src := createMinimalPythonDB(t, dir, "src.db")
	dst := createMinimalPythonDB(t, dir, "dst.db")
	_, err := importer.ImportPythonDB(src, dst, true)
	if err == nil {
		t.Fatal("expected error for python destination")
	}
}

func TestImportRejectsNonemptyGoDestination(t *testing.T) {
	dir := t.TempDir()
	src := createMinimalPythonDB(t, dir, "src.db")
	dst := filepath.Join(dir, "dest.db")
	db, err := storage.Open(dst)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrate.RunAll(db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO feedback(id, role, note, created_at, model_id) VALUES ('x','model','n','2020-01-01T00:00:00Z','test')`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	_, err = importer.ImportPythonDB(src, dst, false)
	if err == nil {
		t.Fatal("expected error for populated go destination")
	}
}

func TestInspectPythonDB(t *testing.T) {
	src := createMinimalPythonDB(t, t.TempDir(), "legacy.db")
	info, err := importer.InspectPythonDB(src)
	if err != nil {
		t.Fatal(err)
	}
	if info.Version != 1 || info.Tables < 5 {
		t.Fatalf("info: %+v", info)
	}
}

func TestInspectPythonDBMissingFile(t *testing.T) {
	_, err := importer.InspectPythonDB(filepath.Join(t.TempDir(), "missing.db"))
	if err == nil {
		t.Fatal("expected open error")
	}
}

func TestImportCreatesDestinationDirectory(t *testing.T) {
	src := createMinimalPythonDB(t, t.TempDir(), "legacy.db")
	dst := filepath.Join(t.TempDir(), "nested", "dir", "development.db")
	if _, err := importer.ImportPythonDB(src, dst, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dst); err != nil {
		t.Fatal(err)
	}
}

func TestImportReplacePopulatedGoDatabase(t *testing.T) {
	dir := t.TempDir()
	src := createMinimalPythonDB(t, dir, "src.db")
	dst := filepath.Join(dir, "dest.db")
	if _, err := importer.ImportPythonDB(src, dst, false); err != nil {
		t.Fatal(err)
	}
	result, err := importer.ImportPythonDB(src, dst, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Tables["feedback"] != 3 {
		t.Fatalf("replace import: %+v", result.Tables)
	}
}

func TestCopyLegacyDataSkipsMissingTables(t *testing.T) {
	dir := t.TempDir()
	src := createMinimalPythonDB(t, dir, "minimal.db")
	dst := filepath.Join(dir, "go.db")
	if _, err := importer.ImportPythonDB(src, dst, false); err != nil {
		t.Fatal(err)
	}
	db, err := storage.Open(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM milestones`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("milestones should be empty when legacy table missing: %d err=%v", n, err)
	}
}

func TestApplyInPlaceRejectsNonPython(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "development.db")
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrate.RunAll(db); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if _, err := importer.ApplyInPlace(path, false, false); err == nil {
		t.Fatal("expected apply error for go schema")
	}
}

func TestCountParityAllTables(t *testing.T) {
	src := createMinimalPythonDB(t, t.TempDir(), "legacy.db")
	db, err := storage.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	got, err := importer.CountParity(db)
	if err != nil {
		t.Fatal(err)
	}
	if got.Feedback != 3 || got.Plans != 1 || got.RepoFiles != 1 || got.VerifyRuns != 1 || got.Archive != 1 {
		t.Fatalf("full parity: %+v", got)
	}
}

func TestImportIntoEmptyGoDatabaseFile(t *testing.T) {
	dir := t.TempDir()
	src := createMinimalPythonDB(t, dir, "legacy.db")
	dst := filepath.Join(dir, "empty-go.db")
	db, err := storage.Open(dst)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrate.RunAll(db); err != nil {
		t.Fatal(err)
	}
	db.Close()

	result, err := importer.ImportPythonDB(src, dst, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Tables["feedback"] != 3 {
		t.Fatalf("feedback rows=%d want 3", result.Tables["feedback"])
	}
	dstDB, err := storage.Open(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer dstDB.Close()
	var closed int
	if err := dstDB.QueryRow(`SELECT COUNT(*) FROM feedback WHERE status='closed'`).Scan(&closed); err != nil || closed != 3 {
		t.Fatalf("mapped closed feedback: %d err=%v", closed, err)
	}
}


func TestImportPythonDBRejectsUnknownSource(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "dest.db")
	unknown := filepath.Join(dir, "unknown.db")
	db, err := storage.Open(unknown)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE schema_migrations (version INTEGER, description TEXT)`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	_, err = importer.ImportPythonDB(unknown, dst, true)
	if err == nil {
		t.Fatal("expected error for unknown schema source")
	}
}
