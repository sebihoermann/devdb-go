package grasscutter

import (
	"os"
	"path/filepath"
	"testing"

	gast "github.com/go-python/gpython/ast"
	"github.com/go-python/gpython/py"
)

func TestReadPythonMissingFile(t *testing.T) {
	repo := t.TempDir()
	pf, err := readPython(repo, "missing.py")
	if err != nil || pf != nil {
		t.Fatalf("missing file: pf=%v err=%v", pf, err)
	}
}

func TestReadPythonInvalidSyntax(t *testing.T) {
	repo := t.TempDir()
	path := filepath.Join(repo, "bad.py")
	if err := os.WriteFile(path, []byte("def (\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pf, err := readPython(repo, "bad.py")
	if err != nil || pf != nil {
		t.Fatalf("invalid syntax: pf=%v err=%v", pf, err)
	}
}

func TestReadPythonDirectoryPath(t *testing.T) {
	repo := t.TempDir()
	dirPath := filepath.Join(repo, "pkg.py")
	if err := os.Mkdir(dirPath, 0o755); err != nil {
		t.Fatal(err)
	}
	pf, err := readPython(repo, "pkg.py")
	if err == nil || pf != nil {
		t.Fatalf("directory path: pf=%v err=%v", pf, err)
	}
}

func TestReadPythonValidModule(t *testing.T) {
	repo := t.TempDir()
	src := "def helper():\n    return 1\n\nhelper()\n"
	if err := os.WriteFile(filepath.Join(repo, "ok.py"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	pf, err := readPython(repo, "ok.py")
	if err != nil || pf == nil || len(walkFunctions(pf.mod)) == 0 {
		t.Fatalf("valid module: pf=%v err=%v", pf, err)
	}
}

func TestFunctionBodyTextAndNormalizeIndent(t *testing.T) {
	repo := t.TempDir()
	text := "def f():\n    x = 1\n    return x\n"
	if err := os.WriteFile(filepath.Join(repo, "body.py"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	mod, err := readPython(repo, "body.py")
	if err != nil || mod == nil {
		t.Fatal(err)
	}
	fd := walkFunctions(mod.mod)[0]
	body := functionBodyText(mod.text, fd)
	if body == "" || body == functionSignature(fd) {
		t.Fatalf("body text: %q", body)
	}
	if normalizeIndent("    a\n\n    b\n") != "a\nb" {
		t.Fatal("normalize indent")
	}
	if normalizeIndent("   \n\t") != "" {
		t.Fatal("blank lines only")
	}
}

func TestReferenceCountsImportsAndCalls(t *testing.T) {
	src := `import os
from pathlib import Path as P
import json as j

def use_refs():
    os.path.join("a")
    P("b")
    j.loads("{}")
    helper()

def helper():
    return 1
`
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "refs.py"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	mod, err := readPython(repo, "refs.py")
	if err != nil || mod == nil {
		t.Fatal(err)
	}
	counts := referenceCounts(mod.mod)
	for _, name := range []string{"os", "P", "j", "helper", "path", "join", "loads"} {
		if counts[name] == 0 {
			t.Fatalf("missing ref count for %s: %+v", name, counts)
		}
	}
}

func TestStmtAndExprSignatures(t *testing.T) {
	name := &gast.Name{Id: "x"}
	assign := &gast.Assign{Value: &gast.Num{N: py.Int(1)}}
	if stmtSignature(assign) == "" {
		t.Fatal("assign signature")
	}
	ret := &gast.Return{Value: name}
	if stmtSignature(ret) == "" {
		t.Fatal("return signature")
	}
	expr := &gast.ExprStmt{Value: &gast.Call{Func: name}}
	if stmtSignature(expr) == "" {
		t.Fatal("expr stmt signature")
	}
	bin := &gast.BinOp{Left: name, Right: name}
	attr := &gast.Attribute{Value: name, Attr: "attr"}
	if exprSignature(bin) == "" || exprSignature(attr) == "" || exprSignature(name) == "" {
		t.Fatal("expr signatures")
	}
	if exprSignature(&gast.Num{N: py.Int(2)}) != "Const" {
		t.Fatal("num signature")
	}
}
