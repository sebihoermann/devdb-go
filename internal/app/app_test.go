package app_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/app"
	"github.com/sebihoermann/devdb-go/internal/migrate"
	"github.com/sebihoermann/devdb-go/internal/storage"
)

func TestOpenMissingDB(t *testing.T) {
	dir := t.TempDir()
	ctx, err := app.Open(dir, filepath.Join(dir, ".devdb", "development.db"), false)
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if ctx.DB != nil {
		t.Fatal("expected nil db for missing file")
	}
}

func TestInitDBAndRequireDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, ".devdb", "development.db")
	ctx, err := app.Open(dir, dbPath, false)
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.InitDB(); err != nil {
		t.Fatal(err)
	}
	if ctx.DB == nil {
		t.Fatal("expected db after init")
	}
	if err := ctx.RequireDB(); err != nil {
		t.Fatal(err)
	}
	kind, _, err := storage.DetectSchema(ctx.DB)
	if err != nil {
		t.Fatal(err)
	}
	if kind != storage.SchemaGo {
		t.Fatalf("kind=%s", kind)
	}
}

func TestRequireDBNotInitialized(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, ".devdb", "development.db")
	ctx, err := app.Open(dir, dbPath, false)
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.RequireDB(); err == nil {
		t.Fatal("expected not initialized error")
	}
}

func TestRequireDBPythonSchema(t *testing.T) {
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

	ctx, err := app.Open(dir, dbPath, false)
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.RequireDB(); err == nil {
		t.Fatal("expected python schema error")
	}
}

func TestOpenModelIDEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DEVDB_MODEL_ID", "test-model")
	ctx, err := app.Open(dir, filepath.Join(dir, "x.db"), false)
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if ctx.ModelID != "test-model" {
		t.Fatalf("model=%q", ctx.ModelID)
	}
}

func TestCloseNilDB(t *testing.T) {
	ctx := &app.Context{}
	if err := ctx.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestOpenJSONMode(t *testing.T) {
	dir := t.TempDir()
	ctx, err := app.Open(dir, filepath.Join(dir, "x.db"), true)
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if !ctx.Out.JSON {
		t.Fatal("expected json mode")
	}
}

func TestOpenExistingDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "existing.db")
	ctx, err := app.Open(dir, dbPath, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := ctx.InitDB(); err != nil {
		t.Fatal(err)
	}
	ctx.Close()

	ctx2, err := app.Open(dir, dbPath, false)
	if err != nil {
		t.Fatal(err)
	}
	defer ctx2.Close()
	if ctx2.DB == nil {
		t.Fatal("expected open db")
	}
}

func TestRequireDBOpensExistingGoDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "existing.db")
	ctx, err := app.Open(dir, dbPath, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := ctx.InitDB(); err != nil {
		t.Fatal(err)
	}
	ctx.Close()

	ctx2, err := app.Open(dir, dbPath, false)
	if err != nil {
		t.Fatal(err)
	}
	defer ctx2.Close()
	ctx2.DB = nil
	if err := ctx2.RequireDB(); err != nil {
		t.Fatal(err)
	}
	if ctx2.DB == nil {
		t.Fatal("expected db opened by RequireDB")
	}
}

func TestRequireDBUnrecognizedSchema(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "weird.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT, description TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO schema_migrations(version, applied_at, description) VALUES (99, datetime('now'), 'custom')`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	ctx, err := app.Open(dir, dbPath, false)
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.RequireDB(); err == nil {
		t.Fatal("expected unrecognized schema error")
	}
}

func TestOpenStatError(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "nope.db")
	if err := os.Mkdir(dbPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Open(dir, dbPath, false); err == nil {
		t.Fatal("expected stat error when db path is directory")
	}
}

func TestInitDBClosesExistingConnection(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, ".devdb", "development.db")
	ctx, err := app.Open(dir, dbPath, false)
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.InitDB(); err != nil {
		t.Fatal(err)
	}
	first := ctx.DB
	if err := ctx.InitDB(); err != nil {
		t.Fatal(err)
	}
	if ctx.DB == nil || ctx.DB == first {
		t.Fatal("expected new db handle after re-init")
	}
}

func TestOpenInvalidSQLiteFile(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "broken.db")
	if err := os.WriteFile(dbPath, []byte("not a database"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Open(dir, dbPath, false); err == nil {
		t.Fatal("expected error opening invalid sqlite file")
	}
}

func TestInitDBMkdirFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("chmod-as-user semantics don't apply under root")
	}
	dir := t.TempDir()
	dbPath := filepath.Join(dir, ".devdb", "development.db")
	ctx, err := app.Open(dir, dbPath, false)
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	if err := ctx.InitDB(); err == nil {
		t.Fatal("expected mkdir failure on read-only repo root")
	}
}

func TestOpenInvalidRepoFlag(t *testing.T) {
	_, err := app.Open(string([]byte{0}), "", false)
	if err == nil {
		t.Fatal("expected resolve error for invalid repo path")
	}
}

func TestRequireDBDetectSchemaError(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "broken.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL, description TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO schema_migrations(version, applied_at, description) VALUES (99, datetime('now'), 'custom')`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	ctx, err := app.Open(dir, dbPath, false)
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.RequireDB(); err == nil {
		t.Fatal("expected unrecognized schema error")
	}
}

func TestRequireDBOpenFailure(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, ".devdb", "development.db")
	ctx, err := app.Open(dir, dbPath, false)
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if ctx.DB != nil {
		t.Fatal("expected nil db before file exists")
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dbPath, []byte{0, 1, 2}, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ctx.RequireDB(); err == nil {
		t.Fatal("expected require db open failure")
	}
}

func TestOpenDefaultModelID(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DEVDB_MODEL_ID", "")
	ctx, err := app.Open(dir, filepath.Join(dir, "x.db"), false)
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if ctx.ModelID != "unknown" {
		t.Fatalf("model=%q", ctx.ModelID)
	}
}

func TestOpenStatPermissionError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("chmod-as-user semantics don't apply under root")
	}
	dir := t.TempDir()
	locked := filepath.Join(dir, "locked")
	if err := os.Mkdir(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	dbPath := filepath.Join(locked, "development.db")
	if _, err := app.Open(dir, dbPath, false); err == nil {
		t.Fatal("expected stat/open error on unreadable parent")
	}
}

func TestInitDBMigrateFailure(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, ".devdb", "development.db")
	ctx, err := app.Open(dir, dbPath, false)
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dbPath, []byte("sqlite format 3\x00"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ctx.InitDB(); err == nil {
		t.Fatal("expected init failure on corrupt sqlite header")
	}
}

func TestRequireDBRunsMigrationsOnOpenDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "partial.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := migrate.SourceMigrations[0].Apply(tx); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL, description TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO schema_migrations(version, applied_at, description) VALUES (1, datetime('now'), 'go:core ledger tables')`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	ctx, err := app.Open(dir, dbPath, false)
	if err != nil {
		t.Fatal(err)
	}
	defer ctx.Close()
	if err := ctx.RequireDB(); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := ctx.DB.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count < 2 {
		t.Fatalf("migrations=%d", count)
	}
}

func TestInitDBOpenPathIsDirectory(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, ".devdb", "development.db")
	if err := os.MkdirAll(dbPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Open(dir, dbPath, false); err == nil {
		t.Fatal("expected open error when db path is a directory")
	}
}

// ensure env default
var _ = os.Getenv
