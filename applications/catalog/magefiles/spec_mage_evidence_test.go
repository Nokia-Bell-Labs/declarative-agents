// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"fmt"
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
			moduleRel, words, ok := parseMageEvidence(evidence)
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
			targets, problems := resolveMageWords(words, available)
			for _, problem := range problems {
				t.Errorf("%s: formal evidence %q %s in %s", entry.Name(), evidence, problem, magefilesDir)
			}
			checked += len(targets)
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

// parseMageEvidence recognizes `mage <words...>` and `cd <rel> && mage
// <words...>` evidence and returns the module directory (relative to the
// catalog module root) the command runs in and the words after `mage`. It
// reports ok=false for any other command, including a `cd ... && go test ...`.
//
// The words are returned unsplit into targets and arguments, because the
// string alone does not say which is which: `mage build test` is two targets
// and `mage diagnose helm-smoke` is a target and its argument. Only the
// discovered targets' arity separates them, which is what resolveMageWords
// does.
func parseMageEvidence(evidence string) (moduleRel string, words []string, ok bool) {
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
	words = strings.Fields(args)
	if len(words) == 0 {
		return "", nil, false
	}
	return moduleRel, words, true
}

// resolveMageWords splits the words after `mage` into targets and their
// arguments, using each discovered target's arity, and returns one problem per
// word it could not account for.
//
// Reading every word as a target made a target that takes an argument
// inexpressible: `mage diagnose helm-smoke` parsed as two targets and failed
// on the second, so the rel17.0 suite dropped the scenario and its formal
// evidence named a command less specific than the one a person runs (GH-2333).
//
// Arity keeps the typo detection the guard exists for. `mage build tset` still
// fails, because Build takes no arguments, so tset has to be a target and is
// not one. Only a target that declares parameters consumes the words after it.
func resolveMageWords(words []string, arity map[string]int) (targets []string, problems []string) {
	for index := 0; index < len(words); {
		name := words[index]
		takes, known := arity[strings.ToLower(name)]
		if !known {
			problems = append(problems, fmt.Sprintf("names Mage target %q, which does not exist", name))
			return targets, problems
		}
		targets = append(targets, name)
		index++
		if remaining := len(words) - index; remaining < takes {
			problems = append(problems, fmt.Sprintf(
				"names Mage target %q, which takes %d argument(s), but supplies %d", name, takes, remaining))
			return targets, problems
		}
		index += takes
	}
	return targets, problems
}

// discoverMageTargets statically parses a magefiles directory and returns each
// Mage target name, lowercased, mapped to the number of arguments it takes:
// each exported top-level function, and each exported method on an mg.Namespace
// type as "namespace:method". Parsing the source rather than running `mage -l`
// keeps the guard hermetic and free of a mage/toolchain dependency.
//
// The arity comes from the same walk because it has to: without it the guard
// cannot tell a second target from the first target's argument (GH-2333).
func discoverMageTargets(dir string) (map[string]int, error) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		return nil, err
	}

	targets := map[string]int{}
	for _, pkg := range pkgs {
		namespaces := namespaceTypeNames(pkg)
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				fn, isFunc := decl.(*ast.FuncDecl)
				if !isFunc || !fn.Name.IsExported() {
					continue
				}
				if fn.Recv == nil {
					targets[strings.ToLower(fn.Name.Name)] = mageTargetArity(fn)
					continue
				}
				if recv := receiverTypeName(fn.Recv); recv != "" && namespaces[recv] {
					targets[strings.ToLower(recv+":"+fn.Name.Name)] = mageTargetArity(fn)
				}
			}
		}
	}
	return targets, nil
}

// mageTargetArity counts the arguments a target takes from the command line.
//
// A leading context.Context does not count: Mage supplies it rather than the
// caller, so `func (Integration) All(ctx context.Context)` is still written as
// `mage integration:all`. Everything else in the signature is a word the
// command line has to carry.
func mageTargetArity(fn *ast.FuncDecl) int {
	if fn.Type.Params == nil {
		return 0
	}
	arity := 0
	for index, field := range fn.Type.Params.List {
		// One field can declare several names (a, b string), and an unnamed
		// parameter declares one.
		count := len(field.Names)
		if count == 0 {
			count = 1
		}
		if index == 0 && count == 1 && isContextType(field.Type) {
			continue
		}
		arity += count
	}
	return arity
}

// isContextType reports whether an expression names context.Context.
func isContextType(expr ast.Expr) bool {
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Context" {
		return false
	}
	ident, ok := selector.X.(*ast.Ident)
	return ok && ident.Name == "context"
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

// The GH-2333 guard. A target's arity is what separates a second target from
// the first target's argument, and the guard has to keep catching a typo while
// letting evidence name the command as run.
func TestResolveMageWordsSeparatesTargetsFromArguments(t *testing.T) {
	t.Parallel()
	arity := map[string]int{"build": 0, "test": 0, "diagnose": 1, "integration:all": 0}
	for name, testCase := range map[string]struct {
		words       []string
		wantTargets []string
		wantProblem string
	}{
		"two targets": {
			words:       []string{"build", "test"},
			wantTargets: []string{"build", "test"},
		},
		"target and its argument": {
			words:       []string{"diagnose", "helm-smoke"},
			wantTargets: []string{"diagnose"},
		},
		"argument does not hide a later target": {
			words:       []string{"diagnose", "helm-smoke", "build"},
			wantTargets: []string{"diagnose", "build"},
		},
		"typo after a target that takes nothing": {
			words:       []string{"build", "tset"},
			wantTargets: []string{"build"},
			wantProblem: `names Mage target "tset", which does not exist`,
		},
		"missing argument": {
			words:       []string{"diagnose"},
			wantTargets: []string{"diagnose"},
			wantProblem: `names Mage target "diagnose", which takes 1 argument(s), but supplies 0`,
		},
		"namespaced target": {
			words:       []string{"integration:all"},
			wantTargets: []string{"integration:all"},
		},
	} {
		targets, problems := resolveMageWords(testCase.words, arity)
		if strings.Join(targets, " ") != strings.Join(testCase.wantTargets, " ") {
			t.Errorf("%s: targets = %v, want %v", name, targets, testCase.wantTargets)
		}
		switch {
		case testCase.wantProblem == "" && len(problems) > 0:
			t.Errorf("%s: problems = %v, want none", name, problems)
		case testCase.wantProblem != "" && len(problems) != 1:
			t.Errorf("%s: problems = %v, want exactly %q", name, problems, testCase.wantProblem)
		case testCase.wantProblem != "" && problems[0] != testCase.wantProblem:
			t.Errorf("%s: problem = %q, want %q", name, problems[0], testCase.wantProblem)
		}
	}
}

// Mage supplies a leading context.Context, so it is not a word the command
// line carries. Counting it would make every context-taking target look like
// it needed an argument.
func TestMageTargetArityIgnoresAContextParameter(t *testing.T) {
	t.Parallel()
	source := `package fixture

import "context"

func Build() error                                { return nil }
func Diagnose(scenario string) error              { return nil }
func Deploy(ctx context.Context) error            { return nil }
func Release(ctx context.Context, tag string)     {}
func Publish(tag, channel string)                 {}
`
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "magefile.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	targets, err := discoverMageTargets(dir)
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]int{
		"build":    0,
		"diagnose": 1,
		"deploy":   0,
		"release":  1,
		"publish":  2,
	} {
		if got, ok := targets[name]; !ok {
			t.Errorf("target %s not discovered", name)
		} else if got != want {
			t.Errorf("%s arity = %d, want %d", name, got, want)
		}
	}
}
