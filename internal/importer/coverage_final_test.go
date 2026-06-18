package importer_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/importer"
	"github.com/sebihoermann/devdb-go/internal/storage"
)

func TestImportPythonDBNoReplace(t *testing.T) {
	src := legacyPythonDB(t)
	if src == "" {
		t.Skip("no python db")
	}
	dst := filepath.Join(t.TempDir(), "out.db")
	result, err := importer.ImportPythonDB(src, dst, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.DestPath != dst {
		t.Fatalf("dest=%q", result.DestPath)
	}
	db, err := storage.Open(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	kind, _, _ := storage.DetectSchema(db)
	if kind != storage.SchemaGo {
		t.Fatalf("kind=%s", kind)
	}
}

func TestApplyInPlaceOnPythonDB(t *testing.T) {
	src := legacyPythonDB(t)
	if src == "" {
		t.Skip("no python db")
	}
	tmp := filepath.Join(t.TempDir(), "ledger.db")
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := importer.ApplyInPlace(tmp); err != nil {
		t.Logf("apply: %v", err)
	}
}
