// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package boundaries

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPackageShapeConstitution(t *testing.T) {
	root := moduleRoot(t)
	base := loadBoundariesBaseline(t, filepath.Join(thisDir(t), "boundaries_baseline.txt"))
	current := collectPackageShapes(t, root)
	violations := packageShapeViolations(current, base.pkgs)
	if len(violations) > 0 {
		t.Errorf("package_shape violations:\n  %s", strings.Join(violations, "\n  "))
	}
}

func TestPackageShapeRejectsOverBaseline(t *testing.T) {
	t.Parallel()
	root := filepath.Join(thisDir(t), "testdata", "shape_violation")
	current := collectPackageShapes(t, root)
	baseline := map[string]pkgShape{"bloated": {files: 1, loc: 1}}
	violations := packageShapeViolations(current, baseline)
	joined := strings.Join(violations, "\n")
	require.Contains(t, joined, "package_shape: bloated")
	require.Contains(t, joined, constitutionPath)
}

func packageShapeViolations(current, baseline map[string]pkgShape) []string {
	var violations []string
	for name, got := range current {
		want, ok := baseline[name]
		if !ok {
			violations = append(violations, "package_shape: "+name+
				" has no baseline entry; see "+constitutionPath)
			continue
		}
		violations = append(violations, shapeGrowthMessages(name, got, want)...)
	}
	for name := range baseline {
		if _, ok := current[name]; !ok {
			violations = append(violations, "package_shape: stale baseline entry "+
				name+"; see "+constitutionPath)
		}
	}
	sort.Strings(violations)
	return violations
}

func shapeGrowthMessages(name string, got, want pkgShape) []string {
	var out []string
	if got.files > want.files {
		out = append(out, fmtShape(name, "files", got.files, want.files))
	}
	if got.loc > want.loc {
		out = append(out, fmtShape(name, "loc", got.loc, want.loc))
	}
	return out
}

func fmtShape(name, metric string, got, want int) string {
	return "package_shape: " + name + " " + metric + " " +
		strconv.Itoa(got) + " exceeds baseline " + strconv.Itoa(want) +
		"; see " + constitutionPath
}

func collectPackageShapes(t *testing.T, root string) map[string]pkgShape {
	t.Helper()
	out := map[string]pkgShape{}
	walkProductionGoFiles(t, root, func(rel, abs string) {
		pkg := filepath.ToSlash(filepath.Dir(rel))
		data, err := os.ReadFile(abs)
		require.NoError(t, err)
		shape := out[pkg]
		shape.files++
		shape.loc += strings.Count(string(data), "\n") + 1
		out[pkg] = shape
	})
	return out
}

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
