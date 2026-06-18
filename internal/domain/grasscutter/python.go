package grasscutter

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	gast "github.com/go-python/gpython/ast"
	"github.com/go-python/gpython/parser"
	"github.com/go-python/gpython/py"
)

type pythonFile struct {
	path string
	text string
	mod  gast.Mod
}

type funcDef struct {
	path    string
	name    string
	line    int
	endLine int
	bodySig string
	node    *gast.FunctionDef
}

func readPython(repoRoot, relPath string) (*pythonFile, error) {
	full := filepath.Join(repoRoot, relPath)
	data, err := os.ReadFile(full)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	text := string(data)
	mod, err := parser.ParseString(text, py.ExecMode)
	if err != nil {
		return nil, nil
	}
	root, ok := mod.(gast.Mod)
	if !ok {
		return nil, nil
	}
	return &pythonFile{path: relPath, text: text, mod: root}, nil
}

func walkFunctions(tree gast.Ast) []*gast.FunctionDef {
	var out []*gast.FunctionDef
	gast.Walk(tree, func(n gast.Ast) bool {
		if fd, ok := n.(*gast.FunctionDef); ok {
			out = append(out, fd)
		}
		return true
	})
	return out
}

func functionEndLine(fd *gast.FunctionDef) int {
	end := fd.GetLineno()
	gast.Walk(fd, func(n gast.Ast) bool {
		if ln := n.GetLineno(); ln > end {
			end = ln
		}
		return true
	})
	return end
}

func functionSignature(fd *gast.FunctionDef) string {
	parts := make([]string, 0, len(fd.Body))
	for _, stmt := range fd.Body {
		parts = append(parts, stmtSignature(stmt))
	}
	return strings.Join(parts, "|")
}

func functionBodyText(text string, fd *gast.FunctionDef) string {
	if len(fd.Body) == 0 {
		return ""
	}
	lines := strings.Split(text, "\n")
	start := fd.Body[0].GetLineno() - 1
	end := functionEndLine(fd)
	if start < 0 || end > len(lines) {
		return functionSignature(fd)
	}
	return normalizeIndent(strings.Join(lines[start:end], "\n"))
}

func normalizeIndent(s string) string {
	raw := strings.Split(s, "\n")
	minIndent := -1
	for _, line := range raw {
		if strings.TrimSpace(line) == "" {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " \t"))
		if minIndent < 0 || indent < minIndent {
			minIndent = indent
		}
	}
	if minIndent < 0 {
		return strings.TrimSpace(s)
	}
	out := make([]string, 0, len(raw))
	for _, line := range raw {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if len(line) >= minIndent {
			line = line[minIndent:]
		}
		out = append(out, strings.TrimRight(line, " \t"))
	}
	return strings.Join(out, "\n")
}

func stmtSignature(stmt gast.Stmt) string {
	switch node := stmt.(type) {
	case *gast.Assign:
		return "Assign:" + exprSignature(node.Value)
	case *gast.Return:
		if node.Value == nil {
			return "Return"
		}
		return "Return:" + exprSignature(node.Value)
	case *gast.ExprStmt:
		return "Expr:" + exprSignature(node.Value)
	default:
		return fmt.Sprintf("%T", stmt)
	}
}

func exprSignature(expr gast.Expr) string {
	switch node := expr.(type) {
	case *gast.Name:
		return "Name:" + string(node.Id)
	case *gast.Num:
		return "Const"
	case *gast.BinOp:
		return "BinOp:" + exprSignature(node.Left) + ":" + exprSignature(node.Right)
	case *gast.Call:
		return "Call:" + exprSignature(node.Func)
	case *gast.Attribute:
		return "Attr:" + exprSignature(node.Value) + "." + string(node.Attr)
	default:
		return fmt.Sprintf("%T", expr)
	}
}

func referenceCounts(tree gast.Ast) map[string]int {
	counts := map[string]int{}
	addName := func(name string) {
		if name != "" {
			counts[name]++
		}
	}
	countRefExpr := func(expr gast.Expr) {
		switch n := expr.(type) {
		case *gast.Name:
			addName(string(n.Id))
		case *gast.Attribute:
			addName(string(n.Attr))
		}
	}
	gast.Walk(tree, func(n gast.Ast) bool {
		switch node := n.(type) {
		case *gast.Call:
			countRefExpr(node.Func)
			for _, arg := range node.Args {
				countRefExpr(arg)
			}
			for _, kw := range node.Keywords {
				if kw != nil && kw.Value != nil {
					countRefExpr(kw.Value)
				}
			}
		case *gast.Attribute:
			addName(string(node.Attr))
		case *gast.ImportFrom:
			for _, alias := range node.Names {
				if alias == nil || string(alias.Name) == "*" {
					continue
				}
				if alias.AsName != "" {
					addName(string(alias.AsName))
				} else {
					addName(string(alias.Name))
				}
			}
		case *gast.Import:
			for _, alias := range node.Names {
				if alias == nil {
					continue
				}
				name := string(alias.Name)
				if alias.AsName != "" {
					addName(string(alias.AsName))
				} else if idx := strings.Index(name, "."); idx >= 0 {
					addName(name[:idx])
				} else {
					addName(name)
				}
			}
		}
		return true
	})
	return counts
}

func deadDefExempt(path string, name string) bool {
	if !strings.HasPrefix(name, "test_") {
		return false
	}
	return strings.HasPrefix(path, "tests/") ||
		strings.HasPrefix(path, "test_") ||
		strings.Contains(path, "/tests/")
}
