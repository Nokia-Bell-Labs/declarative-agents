// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHeavyweightHelpersSkipUnderShort(t *testing.T) {
	t.Parallel()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	dir := filepath.Dir(file)
	requireHelperSkipsShort(t, filepath.Join(dir, "dolt_server_integration_test.go"), "startDoltServer")
	requireHelperSkipsShort(t, filepath.Join(dir, "main_monitor_helpers_test.go"), "startMonitorAgentProcess")
}

func requireHelperSkipsShort(t *testing.T, path, name string) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	require.NoError(t, err)
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != name || fn.Body == nil {
			continue
		}
		require.True(t, functionCallsTestingShort(fn), "%s must skip under testing.Short", name)
		return
	}
	require.Failf(t, "missing helper", "%s in %s", name, path)
}

func functionCallsTestingShort(fn *ast.FuncDecl) bool {
	found := false
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "Short" {
			return true
		}
		pkg, ok := selector.X.(*ast.Ident)
		if ok && pkg.Name == "testing" {
			found = true
			return false
		}
		return true
	})
	return found
}
