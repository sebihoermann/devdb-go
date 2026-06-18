package importer_test

import (
	"path/filepath"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/importer"
	"github.com/sebihoermann/devdb-go/internal/storage"
)

func TestImportPythonDevelopmentDB(t *testing.T) {
	src := legacyPythonDB(t)
	if src == "" {
		t.Skip("no python development.db in repo root")
	}

	srcDB, err := storage.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	want, err := importer.CountParity(srcDB)
	srcDB.Close()
	if err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(t.TempDir(), "development.db")
	result, err := importer.ImportPythonDB(src, dst, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.SourcePath == "" || result.DestPath != dst {
		t.Fatalf("unexpected paths: %+v", result)
	}
	if len(result.Tables) == 0 {
		t.Fatal("expected table copy counts")
	}
	if result.Tables["feedback"] != want.Feedback {
		t.Fatalf("feedback rows=%d want %d", result.Tables["feedback"], want.Feedback)
	}
	if result.Tables["plan_items"] != want.PlanItems {
		t.Fatalf("plan_items rows=%d want %d", result.Tables["plan_items"], want.PlanItems)
	}
	if result.Tables["goals"] != want.Goals {
		t.Fatalf("goals rows=%d want %d", result.Tables["goals"], want.Goals)
	}
	if result.Tables["archive_entries"] != want.Archive {
		t.Fatalf("archive_entries rows=%d want %d", result.Tables["archive_entries"], want.Archive)
	}

	dstDB, err := storage.Open(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer dstDB.Close()

	kind, _, err := storage.DetectSchema(dstDB)
	if err != nil {
		t.Fatal(err)
	}
	if kind != storage.SchemaGo {
		t.Fatalf("kind=%s want go", kind)
	}
	got, err := importer.CountParity(dstDB)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("parity mismatch: got %+v want %+v", got, want)
	}
}

func TestImportRejectsSamePath(t *testing.T) {
	src := legacyPythonDB(t)
	if src == "" {
		t.Skip("no python development.db")
	}
	_, err := importer.ImportPythonDB(src, src, true)
	if err == nil {
		t.Fatal("expected error for same source and destination")
	}
}

func TestImportRejectsNonemptyDestination(t *testing.T) {
	src := legacyPythonDB(t)
	if src == "" {
		t.Skip("no python development.db")
	}
	dir := t.TempDir()
	dst := filepath.Join(dir, "development.db")
	if _, err := importer.ImportPythonDB(src, dst, true); err != nil {
		t.Fatal(err)
	}
	_, err := importer.ImportPythonDB(src, dst, false)
	if err == nil {
		t.Fatal("expected error importing into populated destination without --replace")
	}
}
