// Package budget measures each function of a Go tree against gusto's budget: about 50 lines, and a cyclomatic complexity of
// about 10, which counts one for the function and one for each if, loop, case and && or ||.
package budget

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// The budget a function is held to.
const (
	MaxLines      = 50
	MaxComplexity = 10
)

// Func is one function as it was measured. Name is its file, from the tree's root, and its receiver and name:
// "internal/web/changes.go:(*Server).changes".
type Func struct {
	Name       string
	Lines      int
	Complexity int
}

// Over says whether the function is past the budget.
func (f Func) Over() bool { return f.Lines > MaxLines || f.Complexity > MaxComplexity }

// Measure is every function with a body in the Go files under root, leaving out tests, generated files, and folders that are
// hidden, vendored or test data.
func Measure(root string) ([]Func, error) {
	var funcs []Func
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return skipped(path, root, entry.Name())
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		found, err := measureFile(root, path)
		funcs = append(funcs, found...)
		return err
	})
	return funcs, err
}

// skipped is filepath.SkipDir for a folder whose code is not gusto's own to hold to the budget.
func skipped(path, root, name string) error {
	if path != root && (strings.HasPrefix(name, ".") || name == "vendor" || name == "node_modules" || name == "testdata") {
		return filepath.SkipDir
	}
	return nil
}

func measureFile(root, path string) ([]Func, error) {
	source, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if bytes.Contains(source[:min(len(source), 400)], []byte("Code generated")) {
		return nil, nil
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, source, 0)
	if err != nil {
		return nil, err
	}
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return nil, err
	}
	var funcs []Func
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Body != nil {
			funcs = append(funcs, Func{
				Name:       filepath.ToSlash(relative) + ":" + nameOf(fn),
				Lines:      fset.Position(fn.End()).Line - fset.Position(fn.Pos()).Line + 1,
				Complexity: complexity(fn.Body),
			})
		}
	}
	return funcs, nil
}

// nameOf is a function's name with its receiver's type: "(*Server).changes", or "changes" for a plain function.
func nameOf(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	return fmt.Sprintf("(%s).%s", typeOf(fn.Recv.List[0].Type), fn.Name.Name)
}

func typeOf(expr ast.Expr) string {
	switch x := expr.(type) {
	case *ast.StarExpr:
		return "*" + typeOf(x.X)
	case *ast.IndexExpr:
		return typeOf(x.X)
	case *ast.IndexListExpr:
		return typeOf(x.X)
	case *ast.Ident:
		return x.Name
	}
	return "?"
}

// complexity is one, and one more for each branch in the body, the bodies of the functions written inside it among them.
func complexity(body *ast.BlockStmt) int {
	count := 1
	ast.Inspect(body, func(node ast.Node) bool {
		switch x := node.(type) {
		case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt, *ast.CaseClause, *ast.CommClause:
			count++
		case *ast.BinaryExpr:
			if x.Op == token.LAND || x.Op == token.LOR {
				count++
			}
		}
		return true
	})
	return count
}
