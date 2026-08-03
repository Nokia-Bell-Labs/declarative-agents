// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSpecMageEvidenceTargetsResolve requires every Mage command named as
// formal go_test evidence in the catalog test suites to resolve to a real
// target in the owning module's magefiles. Formal validation skips Mage
// commands (it validates Go symbols, not Mage), so a suite that names a target
// which does not exist — such as a bare `mage uiDist` from a module whose
// magefiles has no uiDist — would otherwise report valid evidence for a command
// that fails with "Unknown target" (GH-1354).
func TestSpecMageEvidenceTargetsResolve(t *testing.T) {
	suitesDir := filepath.Join("..", "docs", "specs", "test-suites")
	entries, err := os.ReadDir(suitesDir)
	if err != nil {
		t.Fatalf("read test-suites: %v", err)
	}

	checked := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(suitesDir, entry.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		for _, evidence := range goTestEvidenceValues(string(data)) {
			moduleRel, targets, ok := parseMageEvidence(evidence)
			if !ok {
				continue
			}
			// moduleRel is relative to the catalog module root; the test runs in
			// its magefiles subdir, so ".." reaches the module root.
			magefilesDir := filepath.Join("..", moduleRel, "magefiles")
			available, err := discoverMageTargets(magefilesDir)
			if err != nil {
				t.Errorf("%s: cannot resolve mage evidence %q: %v", entry.Name(), evidence, err)
				continue
			}
			for _, target := range targets {
				checked++
				if !available[strings.ToLower(target)] {
					t.Errorf("%s: formal evidence %q names Mage target %q, which does not exist in %s",
						entry.Name(), evidence, target, magefilesDir)
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no Mage evidence commands were checked; the guard is a no-op")
	}
}

// goTestEvidenceValues returns the inline value of every `go_test:` line in a
// test-suite document.
func goTestEvidenceValues(doc string) []string {
	var values []string
	for _, line := range strings.Split(doc, "\n") {
		trimmed := strings.TrimSpace(line)
		if v, ok := strings.CutPrefix(trimmed, "go_test:"); ok {
			values = append(values, strings.TrimSpace(v))
		}
	}
	return values
}

// parseMageEvidence recognizes `mage <targets...>` and `cd <rel> && mage
// <targets...>` evidence and returns the module directory (relative to the
// catalog module root) the command runs in and the Mage targets it names. It
// reports ok=false for any other command, including a `cd ... && go test ...`.
func parseMageEvidence(evidence string) (moduleRel string, targets []string, ok bool) {
	command := evidence
	moduleRel = "."
	if rest, isCd := strings.CutPrefix(evidence, "cd "); isCd {
		parts := strings.SplitN(rest, "&&", 2)
		if len(parts) != 2 {
			return "", nil, false
		}
		moduleRel = strings.TrimSpace(parts[0])
		command = strings.TrimSpace(parts[1])
	}
	args, isMage := strings.CutPrefix(command, "mage ")
	if !isMage {
		return "", nil, false
	}
	for _, field := range strings.Fields(args) {
		targets = append(targets, field)
	}
	if len(targets) == 0 {
		return "", nil, false
	}
	return moduleRel, targets, true
}

// discoverMageTargets statically parses a magefiles directory and returns the
// set of Mage target names, lowercased: each exported top-level function, and
// each exported method on an mg.Namespace type as "namespace:method". Parsing
// the source rather than running `mage -l` keeps the guard hermetic and free of
// a mage/toolchain dependency.
func discoverMageTargets(dir string) (map[string]bool, error) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		return nil, err
	}

	targets := map[string]bool{}
	for _, pkg := range pkgs {
		namespaces := namespaceTypeNames(pkg)
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				fn, isFunc := decl.(*ast.FuncDecl)
				if !isFunc || !fn.Name.IsExported() {
					continue
				}
				if fn.Recv == nil {
					targets[strings.ToLower(fn.Name.Name)] = true
					continue
				}
				if recv := receiverTypeName(fn.Recv); recv != "" && namespaces[recv] {
					targets[strings.ToLower(recv+":"+fn.Name.Name)] = true
				}
			}
		}
	}
	return targets, nil
}

// namespaceTypeNames returns the set of type names declared as `type X
// mg.Namespace`, which Mage treats as command namespaces.
func namespaceTypeNames(pkg *ast.Package) map[string]bool {
	names := map[string]bool{}
	for _, file := range pkg.Files {
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.TYPE {
				continue
			}
			for _, spec := range gen.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				sel, ok := ts.Type.(*ast.SelectorExpr)
				if ok && sel.Sel.Name == "Namespace" {
					names[ts.Name.Name] = true
				}
			}
		}
	}
	return names
}

// receiverTypeName returns the (unpointered) receiver type name of a method.
func receiverTypeName(recv *ast.FieldList) string {
	if recv == nil || len(recv.List) != 1 {
		return ""
	}
	switch expr := recv.List[0].Type.(type) {
	case *ast.Ident:
		return expr.Name
	case *ast.StarExpr:
		if ident, ok := expr.X.(*ast.Ident); ok {
			return ident.Name
		}
	}
	return ""
}
