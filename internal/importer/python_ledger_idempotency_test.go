package importer_test

import (
	"errors"
	"io"
	"os"
	"testing"

	"github.com/sebihoermann/devdb-go/internal/importer"
	"github.com/sebihoermann/devdb-go/internal/migrate"
	"github.com/sebihoermann/devdb-go/internal/storage"
)

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// prePlaceGoBak writes a Go-schema sqlite DB at bakPath (typically abs + ".python-bak").
func prePlaceGoBak(t *testing.T, bakPath string) {
	t.Helper()
	db, err := storage.Open(bakPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrate.RunAll(db); err != nil {
		db.Close()
		t.Fatal(err)
	}
	db.Close()
}

func TestApplyWithGoSchemaBakRejects(t *testing.T) {
	dir := t.TempDir()
	srcPath := createPythonDBWithPythonOnlyTables(t, dir)
	bakPath := srcPath + ".python-bak"
	prePlaceGoBak(t, bakPath)

	_, err := importer.ApplyInPlace(srcPath, true, false)
	if !errors.Is(err, importer.ErrPythonBakAlreadyMigrated) {
		t.Fatalf("expected ErrPythonBakAlreadyMigrated, got %v", err)
	}
}

func TestApplyWithGoSchemaBakAndForceSucceeds(t *testing.T) {
	dir := t.TempDir()
	srcPath := createPythonDBWithPythonOnlyTables(t, dir)
	bakPath := srcPath + ".python-bak"
	prePlaceGoBak(t, bakPath)

	if _, err := importer.ApplyInPlace(srcPath, true, true); err != nil {
		t.Fatalf("force should bypass guard: %v", err)
	}
}

func TestApplyWithPythonSchemaBakProceeds(t *testing.T) {
	dir := t.TempDir()
	srcPath := createPythonDBWithPythonOnlyTables(t, dir)
	bakPath := srcPath + ".python-bak"
	if err := copyFile(srcPath, bakPath); err != nil {
		t.Fatal(err)
	}

	if _, err := importer.ApplyInPlace(srcPath, true, false); err != nil {
		t.Fatalf("python-schema bak should not trigger guard: %v", err)
	}
}
