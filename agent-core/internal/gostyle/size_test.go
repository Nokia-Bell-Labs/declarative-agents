// Copyright (c) 2026 Nokia. All rights reserved.

package gostyle

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	maxProductionFileLines = 500
	maxTestFileLines       = 500
	maxFunctionLines       = 40
)

// TestGoStyleSizeConstitution enforces the go-style.yaml size limits
// (functions <= 40 lines, production files <= 500 lines) across the whole
// agent-core module, not just pkg/spec (GH-517). Files and functions that exceed
// the limits today are grandfathered in size_baseline.txt; the gate fails on any
// NEW oversized file or function, and on any stale baseline entry (one that now
// conforms), so the baseline shrinks as the code is split by responsibility.
func TestGoStyleSizeConstitution(t *testing.T) {
	root := moduleRoot(t)
	baseline := loadBaseline(t, filepath.Join(thisDir(t), "size_baseline.txt"))
	seen := map[string]bool{}
	var newViolations []string
	record := func(key string) {
		seen[key] = true
		if !baseline[key] {
			newViolations = append(newViolations, key)
		}
	}

	fset := token.NewFileSet()
	walkProductionGoFiles(t, root, func(rel, abs string) {
		data, err := os.ReadFile(abs)
		require.NoError(t, err)
		if strings.Count(string(data), "\n")+1 > maxProductionFileLines {
			record("file:" + rel)
		}
		f, err := parser.ParseFile(fset, abs, nil, 0)
		require.NoError(t, err)
		ast.Inspect(f, func(n ast.Node) bool {
			fn, ok := n.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				return true
			}
			if fset.Position(fn.End()).Line-fset.Position(fn.Pos()).Line+1 > maxFunctionLines {
				record(fmt.Sprintf("func:%s:%s", rel, fn.Name.Name))
			}
			return true
		})
	})

	var stale []string
	for key := range baseline {
		if !seen[key] {
			stale = append(stale, key)
		}
	}
	sort.Strings(newViolations)
	sort.Strings(stale)
	if len(newViolations) > 0 {
		t.Errorf("new Go size-constitution violations (split by responsibility, or deliberately add to size_baseline.txt):\n  %s",
			strings.Join(newViolations, "\n  "))
	}
	if len(stale) > 0 {
		t.Errorf("stale size_baseline.txt entries now under the limit -- delete them:\n  %s",
			strings.Join(stale, "\n  "))
	}
}

// TestGoTestFileSize enforces focused test ownership. Existing oversized suites
// are temporary, issue-linked debt in test_size_baseline.txt; new oversized test
// files fail immediately, and completed splits make their baseline entries stale.
func TestGoTestFileSize(t *testing.T) {
	root := moduleRoot(t)
	baseline := loadBaseline(t, filepath.Join(thisDir(t), "test_size_baseline.txt"))
	seen := map[string]bool{}
	var newViolations []string
	walkGoFiles(t, root, func(rel, abs string) {
		if !isTestGoFile(rel) {
			return
		}
		data, err := os.ReadFile(abs)
		require.NoError(t, err)
		if strings.Count(string(data), "\n")+1 <= maxTestFileLines {
			return
		}
		key := "file:" + rel
		seen[key] = true
		if !baseline[key] {
			newViolations = append(newViolations, key)
		}
	})
	var stale []string
	for key := range baseline {
		if !seen[key] {
			stale = append(stale, key)
		}
	}
	sort.Strings(newViolations)
	sort.Strings(stale)
	if len(newViolations) > 0 {
		t.Errorf("new oversized Go test files (split by behavior; max %d lines):\n  %s",
			maxTestFileLines, strings.Join(newViolations, "\n  "))
	}
	if len(stale) > 0 {
		t.Errorf("stale test_size_baseline.txt entries now under the limit -- delete them:\n  %s",
			strings.Join(stale, "\n  "))
	}
}

func thisDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Dir(file)
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir := thisDir(t)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, parent, dir, "go.mod not found above the test directory")
		dir = parent
	}
}

func loadBaseline(t *testing.T, path string) map[string]bool {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	out := map[string]bool{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out[line] = true
	}
	require.NoError(t, sc.Err())
	return out
}

// isProductionGoFile reports whether rel is a production Go source file the size
// constitution governs -- excluding tests, generated code, Mage build files, and
// vendored or fixture trees.
func isProductionGoFile(rel string) bool {
	if !strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, "_test.go") || strings.HasSuffix(rel, ".pb.go") {
		return false
	}
	for _, seg := range strings.Split(rel, string(os.PathSeparator)) {
		switch seg {
		case "magefiles", "node_modules", "testdata", "vendor":
			return false
		}
	}
	return true
}

func isTestGoFile(rel string) bool {
	if !strings.HasSuffix(rel, "_test.go") || strings.HasSuffix(rel, ".pb_test.go") {
		return false
	}
	for _, seg := range strings.Split(rel, string(os.PathSeparator)) {
		switch seg {
		case "node_modules", "testdata", "vendor":
			return false
		}
	}
	return true
}

func walkGoFiles(t *testing.T, root string, fn func(rel, abs string)) {
	t.Helper()
	require.NoError(t, filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		fn(rel, path)
		return nil
	}))
}

func walkProductionGoFiles(t *testing.T, root string, fn func(rel, abs string)) {
	t.Helper()
	walkGoFiles(t, root, func(rel, path string) {
		if isProductionGoFile(rel) {
			fn(rel, path)
		}
	})
}
