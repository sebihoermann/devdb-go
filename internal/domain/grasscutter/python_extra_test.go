package grasscutter

import (
	"os"
	"path/filepath"
	"testing"

	gast "github.com/go-python/gpython/ast"
)

func TestFunctionBodyTextEmptyBody(t *testing.T) {
	repo := t.TempDir()
	src := "def empty():\n    pass\n"
	if err := os.WriteFile(filepath.Join(repo, "empty.py"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	pf, err := readPython(repo, "empty.py")
	if err != nil || pf == nil {
		t.Fatal(err)
	}
	fd := walkFunctions(pf.mod)[0]
	if functionBodyText(pf.text, fd) == "" {
		t.Fatal("expected body text for pass")
	}
}

func TestStmtSignatureDefaultBranch(t *testing.T) {
	if stmtSignature(&gast.Pass{}) == "" {
		t.Fatal("expected default stmt signature")
	}
}
