// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"bytes"
	"go/format"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Formatting had no gate at all. The linter policy pins an explicit set with
// default: none, and in the golangci-lint v2 schema formatters are a separate
// section from linters, so no formatting rule was enabled anywhere and two
// unformatted files sat on main unnoticed (GH-1477).
//
// The module configs now declare the gofmt formatter, and this is the check that
// fails a build. It lives here as a test for the same reason the go-style size
// limits do in agent-core internal/gostyle: `mage lint` is not part of the
// release recipe and needs a golangci-lint major version matching the configs,
// while `mage test` runs on every release. Comparing against go/format keeps the
// verdict independent of whichever gofmt binary is on PATH.

// TestEveryModuleIsGofmtClean reports every Go file whose source differs from its
// gofmt form, across the same modules the lint policy covers.
func TestEveryModuleIsGofmtClean(t *testing.T) {
	for _, module := range lintModuleDirs {
		t.Run(module, func(t *testing.T) {
			for _, path := range moduleGoFiles(t, module) {
				assertGofmtClean(t, path)
			}
		})
	}
}

// moduleGoFiles lists the Go files one module owns. A nested module is walked
// under its own entry instead, so no file is judged twice.
func moduleGoFiles(t *testing.T, module string) []string {
	t.Helper()
	root := filepath.Join("..", filepath.FromSlash(module))
	var files []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != root && skipFormatDir(path, entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(entry.Name(), ".go") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// skipFormatDir excludes fixture trees, dependency and build output, and any
// directory that is itself a linted module. Fixtures are excluded because a
// fixture may be deliberately malformed -- the coding agent keeps rejected
// candidate sources under testdata, and a rejected candidate is allowed to be
// ugly.
func skipFormatDir(path, name string) bool {
	switch name {
	case ".git", "node_modules", "testdata", "vendor", "dist", "build":
		return true
	}
	return isLintModule(path)
}

func isLintModule(path string) bool {
	rel, err := filepath.Rel("..", path)
	if err != nil {
		return false
	}
	return slices.Contains(lintModuleDirs, filepath.ToSlash(rel))
}

func assertGofmtClean(t *testing.T, path string) {
	t.Helper()
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	formatted, err := format.Source(source)
	if err != nil {
		t.Fatalf("%s does not parse: %v", path, err)
	}
	if !bytes.Equal(source, formatted) {
		t.Errorf("%s is not gofmt-clean; run: gofmt -w %s", path, path)
	}
}

// TestGofmtGateCoversNestedModulesExactlyOnce pins the walk itself. agent-core
// contains agent-core/magefiles, so without the nested-module skip the inner
// module's files would be judged under both entries, and a future module added
// inside another would silently inherit that.
func TestGofmtGateCoversNestedModulesExactlyOnce(t *testing.T) {
	seen := map[string]string{}
	for _, module := range lintModuleDirs {
		for _, path := range moduleGoFiles(t, module) {
			if owner, duplicate := seen[path]; duplicate {
				t.Errorf("%s is judged by both %s and %s", path, owner, module)
				continue
			}
			seen[path] = module
		}
	}
	if len(seen) == 0 {
		t.Fatal("the gofmt gate found no Go files to judge")
	}
}

// TestGofmtGateRejectsUnformattedSource proves the comparison has teeth, so a
// silent pass cannot come from the check never finding a difference.
func TestGofmtGateRejectsUnformattedSource(t *testing.T) {
	unformatted := []byte("package main\nfunc main()  {\nx := 1\n_ = x\n}\n")
	formatted, err := format.Source(unformatted)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(unformatted, formatted) {
		t.Fatal("go/format accepted deliberately unformatted source")
	}
}
