package importer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/storage"
	"github.com/sebihoermann/devdb-go/internal/testutil"
)

func TestCopyLegacyDataClosedDB(t *testing.T) {
	db := testutil.ClosedDB(t)
	err := copyLegacyData(db, filepath.Join(t.TempDir(), "legacy.db"), map[string]int{})
	if err == nil {
		t.Fatal("expected attach error on closed db")
	}
}

func TestImportPythonDBMkdirFailure(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.db")
	if err := writeMinimalPythonDBWithFeedback(src); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Skip(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	dst := filepath.Join(dir, "nested", "out.db")
	_, err := ImportPythonDB(src, dst, false)
	if err == nil {
		t.Fatal("expected mkdir failure")
	}
}

func TestImportPythonDBOpenDestinationFailure(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.db")
	if err := writeMinimalPythonDBWithFeedback(src); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "dest.db")
	if err := os.Mkdir(dst, 0o444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dst, 0o755); _ = os.Remove(dst) })
	_, err := ImportPythonDB(src, dst, true)
	if err == nil {
		t.Fatal("expected open destination failure")
	}
}

func TestInspectPythonDBNotLegacy(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.db")
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	_, err = InspectPythonDB(path)
	if err == nil {
		t.Fatal("expected not legacy error")
	}
}

func TestFileExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "exists.db")
	if fileExists(path) {
		t.Fatal("should not exist yet")
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !fileExists(path) {
		t.Fatal("should exist")
	}
}

func TestCountParityClosedDBBoost(t *testing.T) {
	db := testutil.ClosedDB(t)
	if _, err := CountParity(db); err == nil {
		t.Fatal("expected parity error on closed db")
	}
}
