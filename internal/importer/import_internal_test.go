package importer

import (
	"path/filepath"
	"testing"
)

func TestImportPythonDBCreatesNestedDestination(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "legacy.db")
	dst := filepath.Join(dir, "nested", "dir", "development.db")
	if err := writeMinimalPythonDBWithFeedback(src); err != nil {
		t.Fatal(err)
	}
	if _, err := ImportPythonDB(src, dst, false); err != nil {
		t.Fatal(err)
	}
}
