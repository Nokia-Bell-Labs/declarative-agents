// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package boundaries

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const constitutionPath = "docs/constitutions/boundaries.yaml"

type goListPackage struct {
	ImportPath string
	Imports    []string
	Dir        string
}

type boundariesBaseline struct {
	imports map[string]bool
	pkgs    map[string]pkgShape
}

type pkgShape struct {
	files int
	loc   int
}

func TestImportLawConstitution(t *testing.T) {
	root := moduleRoot(t)
	base := loadBoundariesBaseline(t, filepath.Join(thisDir(t), "boundaries_baseline.txt"))
	violations := importLawViolations(t, listModulePackages(t, root), modulePath(t, root), base.imports)
	if len(violations) > 0 {
		t.Errorf("import_law violations:\n  %s", strings.Join(violations, "\n  "))
	}
}

func TestImportLawRejectsForbiddenImport(t *testing.T) {
	t.Parallel()
	root := filepath.Join(thisDir(t), "testdata", "import_violation")
	violations := importLawViolations(t, listModulePackages(t, root), modulePath(t, root), nil)
	joined := strings.Join(violations, "\n")
	want := "import_law: internal/runtime/core imports database/sql; see " + constitutionPath
	require.Contains(t, joined, want)
}

func importLawViolations(t *testing.T, pkgs []goListPackage, module string, allowed map[string]bool) []string {
	t.Helper()
	seen := map[string]bool{}
	var violations []string
	for _, pkg := range pkgs {
		from := relImport(module, pkg.ImportPath)
		for _, imp := range pkg.Imports {
			to := relImport(module, imp)
			key := "import:" + from + " -> " + to
			if msg := importLawMessage(from, to); msg != "" {
				seen[key] = true
				if allowed[key] {
					continue
				}
				violations = append(violations, msg)
			}
		}
	}
	violations = append(violations, staleImportAllowances(allowed, seen)...)
	sort.Strings(violations)
	return violations
}

func importLawMessage(from, to string) string {
	switch {
	case isRuntimePkg(from) && isToolsPkg(to):
		return importLawMsg(from, to)
	case from == "internal/runtime/core" && isForbiddenCoreStorage(to):
		return importLawMsg(from, to)
	case isObservabilityPkg(from) && (isToolsPkg(to) || isRuntimePkg(to)):
		return importLawMsg(from, to)
	case isPublicPkg(from) && isInternalPkg(to):
		return importLawMsg(from, to)
	default:
		return ""
	}
}

func importLawMsg(from, to string) string {
	return fmt.Sprintf("import_law: %s imports %s; see %s", from, to, constitutionPath)
}

func staleImportAllowances(allowed, seen map[string]bool) []string {
	var stale []string
	for key := range allowed {
		if !strings.HasPrefix(key, "import:") || seen[key] {
			continue
		}
		stale = append(stale, "import_law: stale baseline allowance "+key+"; see "+constitutionPath)
	}
	return stale
}

func isRuntimePkg(p string) bool {
	return p == "internal/runtime" || strings.HasPrefix(p, "internal/runtime/")
}

func isToolsPkg(p string) bool {
	return p == "internal/tools" || strings.HasPrefix(p, "internal/tools/")
}

func isObservabilityPkg(p string) bool {
	return p == "internal/observability" || strings.HasPrefix(p, "internal/observability/")
}

func isPublicPkg(p string) bool {
	return p == "pkg" || strings.HasPrefix(p, "pkg/")
}

func isInternalPkg(p string) bool {
	return p == "internal" || strings.HasPrefix(p, "internal/")
}

func isForbiddenCoreStorage(imp string) bool {
	switch {
	case imp == "database/sql", strings.HasPrefix(imp, "database/sql/"):
		return true
	case strings.Contains(imp, "go-sql-driver"), strings.Contains(imp, "dolthub"):
		return true
	default:
		return false
	}
}

func relImport(module, importPath string) string {
	if importPath == module {
		return "."
	}
	prefix := module + "/"
	if strings.HasPrefix(importPath, prefix) {
		return strings.TrimPrefix(importPath, prefix)
	}
	return importPath
}

func listModulePackages(t *testing.T, dir string) []goListPackage {
	t.Helper()
	cmd := exec.Command("go", "list", "-deps", "-json", "./...")
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	require.NoErrorf(t, err, "go list -deps -json ./... in %s: %s", dir, stderr.String())
	return decodeGoList(t, out, modulePath(t, dir), dir)
}

func decodeGoList(t *testing.T, data []byte, module, root string) []goListPackage {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(data))
	var pkgs []goListPackage
	for {
		var pkg goListPackage
		err := dec.Decode(&pkg)
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		if pkg := keepListedPackage(pkg, module, root); pkg != nil {
			pkgs = append(pkgs, *pkg)
		}
	}
	return pkgs
}

func keepListedPackage(pkg goListPackage, module, root string) *goListPackage {
	if !strings.HasPrefix(pkg.ImportPath, module) {
		return nil
	}
	rel, err := filepath.Rel(root, pkg.Dir)
	if err != nil || skipListedRel(rel) {
		return nil
	}
	return &pkg
}

func skipListedRel(rel string) bool {
	for _, seg := range strings.Split(filepath.ToSlash(rel), "/") {
		switch seg {
		case "testdata", "magefiles", "vendor", "node_modules":
			return true
		}
	}
	return false
}

func modulePath(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("go", "list", "-m")
	cmd.Dir = dir
	out, err := cmd.Output()
	require.NoError(t, err)
	return strings.TrimSpace(string(out))
}

func thisDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Dir(file)
}

func loadBoundariesBaseline(t *testing.T, path string) boundariesBaseline {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	out := boundariesBaseline{imports: map[string]bool{}, pkgs: map[string]pkgShape{}}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		parseBaselineLine(t, strings.TrimSpace(sc.Text()), &out)
	}
	require.NoError(t, sc.Err())
	return out
}

func parseBaselineLine(t *testing.T, line string, out *boundariesBaseline) {
	t.Helper()
	if line == "" || strings.HasPrefix(line, "#") {
		return
	}
	switch {
	case strings.HasPrefix(line, "import:"):
		out.imports[line] = true
	case strings.HasPrefix(line, "pkg:"):
		name, shape := parsePkgShapeLine(t, line)
		out.pkgs[name] = shape
	default:
		t.Fatalf("unrecognized baseline line %q", line)
	}
}

func parsePkgShapeLine(t *testing.T, line string) (string, pkgShape) {
	t.Helper()
	rest := strings.TrimPrefix(line, "pkg:")
	parts := strings.Fields(rest)
	require.Len(t, parts, 3, "pkg baseline line %q", line)
	files := baselineIntField(t, parts[1], "files=")
	loc := baselineIntField(t, parts[2], "loc=")
	return parts[0], pkgShape{files: files, loc: loc}
}

func baselineIntField(t *testing.T, field, prefix string) int {
	t.Helper()
	require.True(t, strings.HasPrefix(field, prefix), "field %q prefix %q", field, prefix)
	n, err := strconv.Atoi(strings.TrimPrefix(field, prefix))
	require.NoError(t, err)
	return n
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
