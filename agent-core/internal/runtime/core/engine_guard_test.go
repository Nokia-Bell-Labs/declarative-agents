// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestCoreSourceDoesNotEmbedModeOrToolPolicyNames(t *testing.T) {
	t.Parallel()

	forbidden := map[string]struct{}{
		"generator":            {},
		"planner":              {},
		"evaluator":            {},
		"bench":                {},
		"jurist": {},
		"invoke_llm":           {},
		"parse_response":       {},
		"report_parse_error":   {},
		"parse_plan":           {},
		"run_point":            {},
		"serve_ui":             {},
	}

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current file")
	}
	dir := filepath.Dir(currentFile)
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	for _, file := range files {
		name := file.Name()
		if file.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(dir, name)
		fset := token.NewFileSet()
		parsed, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		ast.Inspect(parsed, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			value, err := strconv.Unquote(lit.Value)
			if err != nil {
				t.Fatalf("unquote %s in %s: %v", lit.Value, name, err)
			}
			if _, exists := forbidden[value]; exists {
				pos := fset.Position(lit.Pos())
				t.Fatalf("internal/runtime/core must not embed mode/tool policy literal %q at %s; put workflow policy in machine/tool config", value, pos)
			}
			return true
		})
	}
}
