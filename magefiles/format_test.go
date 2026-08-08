// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"io/fs"
	"os"
	"os/exec"
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
// The module configs declare the gofmt formatter, and this is the check that
// fails a build. It lives here as a test for the same reason the go-style size
// limits do in agent-core internal/gostyle: `mage lint` is not part of the
// release recipe and needs a golangci-lint major version matching the configs,
// while `mage test` runs on every release.
//
// It runs the gofmt shipped with the Go toolchain that compiles this test, found
// through GOROOT rather than PATH, so the verdict tracks the toolchain instead of
// whichever gofmt happens to come first. -s is passed because the golangci-lint
// gofmt formatter simplifies by default, and a gate weaker than the policy it
// claims to enforce is worse than no gate: the first version of this check
// compared against go/format, which has no simplify pass, and it silently
// accepted a file the declared policy rejects (GH-1479).

// TestEveryModuleIsGofmtClean reports every Go file gofmt would rewrite, across
// the same modules the lint policy covers.
func TestEveryModuleIsGofmtClean(t *testing.T) {
	gofmt := toolchainGofmt(t)
	for _, module := range lintModuleDirs {
		t.Run(module, func(t *testing.T) {
			files := moduleGoFiles(t, module)
			if len(files) == 0 {
				t.Fatalf("%s has no Go files to judge", module)
			}
			for _, path := range gofmtWouldRewrite(t, gofmt, files) {
				t.Errorf("%s is not gofmt-clean; run: gofmt -s -w %s", path, path)
			}
		})
	}
}

// toolchainGofmt returns the gofmt belonging to the Go toolchain in use.
func toolchainGofmt(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		t.Fatalf("go env GOROOT: %v", err)
	}
	path := filepath.Join(strings.TrimSpace(string(out)), "bin", "gofmt")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the Go toolchain has no gofmt at %s: %v", path, err)
	}
	return path
}

// gofmtWouldRewrite lists the files gofmt -s would change. -l prints exactly
// those paths and nothing for clean input.
func gofmtWouldRewrite(t *testing.T, gofmt string, files []string) []string {
	t.Helper()
	out, err := exec.Command(gofmt, append([]string{"-s", "-l"}, files...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("gofmt -s -l: %v\n%s", err, out)
	}
	var listed []string
	for _, line := range strings.Split(string(out), "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			listed = append(listed, trimmed)
		}
	}
	return listed
}

// moduleGoFiles lists the Go files one module owns. A nested module is walked
// under its own entry instead, so no file is judged twice.
func moduleGoFiles(t *testing.T, module string) []string {
	t.Helper()
	repoRoot, err := findRepositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(repoRoot, filepath.FromSlash(module))
	var files []string
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
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
	root, err := findRepositoryRoot()
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return slices.Contains(lintModuleDirs, filepath.ToSlash(rel))
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

// TestGofmtGateSimplifies is the regression guard for GH-1479. A composite
// literal repeating its element type is formatted correctly but not simplified:
// plain gofmt accepts it and gofmt -s rewrites it. The gate must reject it,
// because the module configs enable the formatter with simplify on. Asserting
// both halves is what pins the gate to the declared policy rather than to
// whichever is more convenient.
func TestGofmtGateSimplifies(t *testing.T) {
	source := "package fixture\n\ntype pair struct{ a int }\n\nvar pairs = []pair{pair{a: 1}}\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "fixture.go")
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	gofmt := toolchainGofmt(t)

	if rewritten := gofmtWouldRewrite(t, gofmt, []string{path}); len(rewritten) == 0 {
		t.Error("gofmt -s accepted a composite literal that repeats its element type")
	}
	plain, err := exec.Command(gofmt, "-l", path).CombinedOutput()
	if err != nil {
		t.Fatalf("gofmt -l: %v\n%s", err, plain)
	}
	if strings.TrimSpace(string(plain)) != "" {
		t.Errorf("the fixture is meant to be gofmt-clean without -s, but gofmt -l listed it:\n%s", plain)
	}
}
