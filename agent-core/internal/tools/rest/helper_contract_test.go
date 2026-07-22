// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTestingHelpersAttributeFailuresToCallers(t *testing.T) {
	t.Parallel()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	restDir := filepath.Dir(file)
	coreRoot := filepath.Clean(filepath.Join(restDir, "..", "..", ".."))
	restTests, err := filepath.Glob(filepath.Join(restDir, "*_test.go"))
	require.NoError(t, err)
	docsTests, err := filepath.Glob(filepath.Join(
		coreRoot, "internal", "knowledge", "documentation", "host_*_test.go",
	))
	require.NoError(t, err)
	require.NotEmpty(t, docsTests)
	files := append(restTests, docsTests...)

	var missing []string
	for _, path := range files {
		missing = append(missing, helpersMissingAttribution(t, path)...)
	}
	require.Empty(t, missing, "testing helpers must call t.Helper() as their first statement")
}

func helpersMissingAttribution(t *testing.T, path string) []string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	require.NoError(t, err)
	var missing []string
	for _, declaration := range file.Decls {
		fn, ok := declaration.(*ast.FuncDecl)
		if !ok || fn.Body == nil || isTestingEntrypoint(fn.Name.Name) {
			continue
		}
		handles := testingHandleNames(fn.Type.Params)
		if len(handles) == 0 {
			continue
		}
		if !firstStatementMarksHelper(fn.Body, handles) {
			position := fset.Position(fn.Pos())
			missing = append(missing, position.String()+" "+fn.Name.Name)
		}
	}
	return missing
}

func isTestingEntrypoint(name string) bool {
	return strings.HasPrefix(name, "Test") ||
		strings.HasPrefix(name, "Fuzz") ||
		strings.HasPrefix(name, "Benchmark")
}

func testingHandleNames(fields *ast.FieldList) map[string]bool {
	handles := map[string]bool{}
	if fields == nil {
		return handles
	}
	for _, field := range fields.List {
		if !isTestingHandle(field.Type) {
			continue
		}
		for _, name := range field.Names {
			handles[name.Name] = true
		}
	}
	return handles
}

func isTestingHandle(expression ast.Expr) bool {
	if pointer, ok := expression.(*ast.StarExpr); ok {
		expression = pointer.X
	}
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok || (selector.Sel.Name != "T" && selector.Sel.Name != "TB") {
		return false
	}
	pkg, ok := selector.X.(*ast.Ident)
	return ok && pkg.Name == "testing"
}

func firstStatementMarksHelper(body *ast.BlockStmt, handles map[string]bool) bool {
	if len(body.List) == 0 {
		return false
	}
	statement, ok := body.List[0].(*ast.ExprStmt)
	if !ok {
		return false
	}
	call, ok := statement.X.(*ast.CallExpr)
	if !ok || len(call.Args) != 0 {
		return false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Helper" {
		return false
	}
	handle, ok := selector.X.(*ast.Ident)
	return ok && handles[handle.Name]
}
