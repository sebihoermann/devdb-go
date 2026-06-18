package hub

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestOpenHubAndSyncOne(t *testing.T) {
	metaDB := filepath.Join(t.TempDir(), "hub", "metadata.db")
	hubDB, err := OpenHub(metaDB)
	if err != nil {
		t.Fatal(err)
	}
	defer hubDB.Close()

	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	dbPath := filepath.Join(dir, ".devdb", "development.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	srcDB, path := testutil.TempDB(t)
	_ = srcDB.Close()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dbPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ok, msg := SyncOne(hubDB, RegisteredProject{Alias: "p1", Root: repo, DBPath: dbPath, Exists: true})
	if !ok || msg != "" {
		t.Fatalf("sync one: ok=%v msg=%q", ok, msg)
	}

	raw, err := encodeSnapshot(Snapshot{SyncStatus: "active"})
	if err != nil || raw == "" {
		t.Fatalf("encode: %q err=%v", raw, err)
	}
	dec, err := decodeSnapshot(raw)
	if err != nil || dec.SyncStatus != "active" {
		t.Fatalf("decode: %+v err=%v", dec, err)
	}
	empty, _ := decodeSnapshot("")
	if empty.SyncStatus != "" {
		t.Fatal("empty decode")
	}
}

func TestMarkDirtyAndRegisterFlow(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	dbPath := filepath.Join(dir, ".devdb", "development.db")
	registry := filepath.Join(dir, "registry.json")
	metaDB := filepath.Join(dir, "metadata.db")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.TempDB(t) // ensure migrations exist
	srcDB, srcPath := testutil.TempDB(t)
	_ = srcDB.Close()
	data, _ := os.ReadFile(srcPath)
	_ = os.MkdirAll(filepath.Dir(dbPath), 0o755)
	if err := os.WriteFile(dbPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Register(repo, "regtest", registry, metaDB); err != nil {
		t.Fatal(err)
	}
	MarkDirty(dbPath)
	entries, err := List(registry, metaDB, true)
	if err != nil || len(entries) != 1 {
		t.Fatalf("list refresh: %d err=%v", len(entries), err)
	}
}

func TestSyncOneMissingDB(t *testing.T) {
	hubDB, err := OpenHub(filepath.Join(t.TempDir(), "meta.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer hubDB.Close()
	ok, _ := SyncOne(hubDB, RegisteredProject{
		Alias: "missing", Root: t.TempDir(),
		DBPath: filepath.Join(t.TempDir(), "nope.db"), Exists: false,
	})
	if ok {
		t.Fatal("missing project should not sync ok")
	}
}
