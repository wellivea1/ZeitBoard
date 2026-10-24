// Package codequality holds the Go code to the limits the window's lint holds
// its TypeScript to (eslint.config.js): a function's cyclomatic complexity at
// most 24, and its length at most 300 lines. Each module's tests run Over on
// that module's own source, so a function past either limit fails its
// module's checks, as it would fail the lint.
package codequality

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

const (
	MaxComplexity = 24
	MaxLines      = 300
)

// Function is one function's measures.
type Function struct {
	Name       string
	File       string
	Line       int
	Complexity int
	Lines      int
}

// skipped are folders that hold no Go of the module's own: test data, the
// window's frontend and its packages, and build output.
var skipped = map[string]bool{"testdata": true, "frontend": true, "node_modules": true, "build": true}

// Measure measures every function and method declared in the Go files under
// root, tests excluded, in file order.
func Measure(root string) ([]Function, error) {
	var functions []Function
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != root && (skipped[entry.Name()] || strings.HasPrefix(entry.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		measured, err := measureFile(path)
		functions = append(functions, measured...)
		return err
	})
	return functions, err
}

func measureFile(path string) ([]Function, error) {
	files := token.NewFileSet()
	parsed, err := parser.ParseFile(files, path, nil, 0)
	if err != nil {
		return nil, err
	}
	var functions []Function
	for _, declaration := range parsed.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Body == nil {
			continue
		}
		start := files.Position(function.Pos()).Line
		functions = append(functions, Function{
			Name:       name(function),
			File:       filepath.ToSlash(path),
			Line:       start,
			Complexity: Complexity(function.Body),
			Lines:      files.Position(function.End()).Line - start + 1,
		})
	}
	return functions, nil
}

// name is a function's name, with its receiver's type for a method.
func name(function *ast.FuncDecl) string {
	if function.Recv == nil || len(function.Recv.List) == 0 {
		return function.Name.Name
	}
	receiver := function.Recv.List[0].Type
	if star, ok := receiver.(*ast.StarExpr); ok {
		receiver = star.X
	}
	if generic, ok := receiver.(*ast.IndexExpr); ok {
		receiver = generic.X
	}
	if ident, ok := receiver.(*ast.Ident); ok {
		return ident.Name + "." + function.Name.Name
	}
	return function.Name.Name
}

// Complexity counts a body's decisions as gocyclo does: one for the body,
// and one for each if, for, range, case and select case but the default,
// and each && and ||. A closure's decisions count toward the function that
// holds it.
func Complexity(body ast.Node) int {
	count := 1
	ast.Inspect(body, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt:
			count++
		case *ast.CaseClause:
			if n.List != nil {
				count++
			}
		case *ast.CommClause:
			if n.Comm != nil {
				count++
			}
		case *ast.BinaryExpr:
			if n.Op == token.LAND || n.Op == token.LOR {
				count++
			}
		}
		return true
	})
	return count
}

// Over is every function under root past either limit, the most complex
// first.
func Over(root string) ([]Function, error) {
	functions, err := Measure(root)
	if err != nil {
		return nil, err
	}
	var over []Function
	for _, function := range functions {
		if function.Complexity > MaxComplexity || function.Lines > MaxLines {
			over = append(over, function)
		}
	}
	sort.SliceStable(over, func(i, j int) bool { return over[i].Complexity > over[j].Complexity })
	return over, nil
}
