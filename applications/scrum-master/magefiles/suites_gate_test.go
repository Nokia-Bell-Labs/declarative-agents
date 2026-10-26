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
	"regexp"
	"regexp/syntax"
	"slices"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/corpus"
)

// evidence resolves the commands a test suite or the road map names against
// the repository: the top-level tests of each Go package, read from its
// sources, and the mage targets the magefiles define.
type evidence struct {
	root    string
	tests   map[string][]string // package directory -> test names
	targets map[string]bool     // lower-case mage target names
}

func newEvidence(root string) (*evidence, error) {
	e := &evidence{root: root, tests: map[string][]string{}, targets: map[string]bool{}}
	files, err := parseDir(filepath.Join(root, "magefiles"), false)
	if err != nil {
		return nil, err
	}
	namespaces := map[string]bool{}
	for _, f := range files {
		for _, d := range f.Decls {
			if g, ok := d.(*ast.GenDecl); ok && g.Tok == token.TYPE {
				for _, s := range g.Specs {
					if sel, ok := s.(*ast.TypeSpec).Type.(*ast.SelectorExpr); ok && sel.Sel.Name == "Namespace" {
						namespaces[s.(*ast.TypeSpec).Name.Name] = true
					}
				}
			}
		}
	}
	for _, f := range files {
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || !fn.Name.IsExported() {
				continue
			}
			switch {
			case fn.Recv == nil:
				e.targets[strings.ToLower(fn.Name.Name)] = true
			case len(fn.Recv.List) == 1:
				if id, ok := fn.Recv.List[0].Type.(*ast.Ident); ok && namespaces[id.Name] {
					e.targets[strings.ToLower(id.Name+":"+fn.Name.Name)] = true
				}
			}
		}
	}
	return e, nil
}

// parseDir parses the Go files of dir, only the test files when tests is set
// and only the others when it is not, whatever their build constraints.
func parseDir(dir string, tests bool) ([]*ast.File, error) {
	names, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return nil, err
	}
	var out []*ast.File
	for _, n := range names {
		if strings.HasSuffix(n, "_test.go") != tests {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), n, nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, nil
}

func (e *evidence) testsIn(pkg string) ([]string, error) {
	if names, ok := e.tests[pkg]; ok {
		return names, nil
	}
	dir := filepath.Join(e.root, pkg)
	if _, err := os.Stat(dir); err != nil {
		return nil, fmt.Errorf("package %s does not exist", pkg)
	}
	files, err := parseDir(dir, true)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, f := range files {
		for _, d := range f.Decls {
			if fn, ok := d.(*ast.FuncDecl); ok && fn.Recv == nil && strings.HasPrefix(fn.Name.Name, "Test") && fn.Name.Name != "TestMain" {
				names = append(names, fn.Name.Name)
			}
		}
	}
	e.tests[pkg] = names
	return names, nil
}

// check returns what is wrong with one command, or "" when it resolves. A
// command other than go test or mage is prose and is not checked.
func (e *evidence) check(cmd string) string {
	fields := strings.Fields(cmd)
	switch {
	case len(fields) == 2 && fields[0] == "mage":
		if !e.targets[strings.ToLower(fields[1])] {
			return fmt.Sprintf("%q: the magefiles define no target %s", cmd, fields[1])
		}
	case len(fields) > 2 && fields[0] == "go" && fields[1] == "test":
		return e.checkGoTest(cmd, fields[2:])
	}
	return ""
}

func (e *evidence) checkGoTest(cmd string, args []string) string {
	var pkgs []string
	pattern := ""
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "-run" && i+1 < len(args):
			pattern = strings.Trim(args[i+1], `'"`)
			i++
		case strings.HasPrefix(args[i], "./"):
			pkgs = append(pkgs, args[i])
		}
	}
	var tests []string
	for _, p := range pkgs {
		names, err := e.testsIn(p)
		if err != nil {
			return fmt.Sprintf("%q: %v", cmd, err)
		}
		tests = append(tests, names...)
	}
	if pattern == "" {
		return ""
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Sprintf("%q: %v", cmd, err)
	}
	// An anchored pattern without repetition names its tests exactly; each
	// must exist. Any other pattern must match at least one test.
	if names, ok := namedTests(pattern); ok {
		var missing []string
		for _, n := range names {
			if !slices.Contains(tests, n) {
				missing = append(missing, n)
			}
		}
		if len(missing) > 0 {
			return fmt.Sprintf("%q: no test %s in %s", cmd, strings.Join(missing, ", "), strings.Join(pkgs, " "))
		}
		return ""
	}
	if !slices.ContainsFunc(tests, re.MatchString) {
		return fmt.Sprintf("%q: -run matches no test in %s", cmd, strings.Join(pkgs, " "))
	}
	return ""
}

// namedTests returns every name an anchored -run pattern without repetition
// matches, such as ^TestPop(A|B)$.
func namedTests(pattern string) ([]string, bool) {
	if !strings.HasPrefix(pattern, "^") || !strings.HasSuffix(pattern, "$") {
		return nil, false
	}
	re, err := syntax.Parse(pattern, syntax.Perl)
	if err != nil {
		return nil, false
	}
	return language(re)
}

// language enumerates the strings re matches, and reports false when they
// are not a small finite set.
func language(re *syntax.Regexp) ([]string, bool) {
	switch re.Op {
	case syntax.OpEmptyMatch, syntax.OpBeginLine, syntax.OpEndLine, syntax.OpBeginText, syntax.OpEndText:
		return []string{""}, true
	case syntax.OpLiteral:
		return []string{string(re.Rune)}, true
	case syntax.OpCharClass:
		var out []string
		for i := 0; i+1 < len(re.Rune); i += 2 {
			for r := re.Rune[i]; r <= re.Rune[i+1]; r++ {
				if out = append(out, string(r)); len(out) > 64 {
					return nil, false
				}
			}
		}
		return out, true
	case syntax.OpCapture:
		return language(re.Sub[0])
	case syntax.OpAlternate:
		var out []string
		for _, s := range re.Sub {
			l, ok := language(s)
			if !ok {
				return nil, false
			}
			out = append(out, l...)
		}
		return out, true
	case syntax.OpConcat:
		out := []string{""}
		for _, s := range re.Sub {
			l, ok := language(s)
			if !ok {
				return nil, false
			}
			var next []string
			for _, a := range out {
				for _, b := range l {
					next = append(next, a+b)
				}
			}
			if out = next; len(out) > 1024 {
				return nil, false
			}
		}
		return out, true
	}
	return nil, false
}

// suiteProblems checks an implemented test suite's go_test and
// integration_target commands; a suite still specified names tests and
// targets its release has yet to build, and a retired case is skipped.
func (e *evidence) suiteProblems(doc *corpus.Document) []string {
	if doc.Front["status"] != "implemented" {
		return nil
	}
	var out []string
	for _, c := range doc.Records("case") {
		// A retired case names evidence its feature no longer has (GH-37).
		if c.Attrs["status"] == "retired" {
			continue
		}
		for _, f := range []string{"go_test", "integration_target"} {
			for _, line := range strings.Split(c.Field(f).Text(), "\n") {
				if p := e.check(strings.TrimSpace(line)); p != "" {
					out = append(out, fmt.Sprintf("%s %s %s: %s", doc.ID(), c.Key, f, p))
				}
			}
		}
	}
	return out
}

// roadMapProblems checks the proof list of each implemented release.
func (e *evidence) roadMapProblems(doc *corpus.Document) []string {
	var out []string
	for _, r := range doc.Records("release") {
		if r.Attrs["status"] != "implemented" {
			continue
		}
		for _, item := range r.Field("proof").List() {
			if p := e.check(item); p != "" {
				out = append(out, fmt.Sprintf("road map %s proof: %s", r.Key, p))
			}
		}
	}
	return out
}

func readDoc(t *testing.T, path string) *corpus.Document {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := corpus.ParseMarkdown(b)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return doc
}

// TestSuitesNameRealTests holds each implemented test suite and release to
// evidence that runs (srd040 R2.3): every Go test a suite's go_test names
// exists in its package, and every mage target a suite or the road map's
// proof names is defined in the magefiles.
func TestSuitesNameRealTests(t *testing.T) {
	e, err := newEvidence("..")
	if err != nil {
		t.Fatal(err)
	}
	suites, err := filepath.Glob(filepath.Join("..", "docs", "specs", "test-suites", "*.md"))
	if err != nil || len(suites) == 0 {
		t.Fatalf("no test suites: %v", err)
	}
	var problems []string
	for _, s := range suites {
		problems = append(problems, e.suiteProblems(readDoc(t, s))...)
	}
	problems = append(problems, e.roadMapProblems(readDoc(t, filepath.Join("..", "docs", "road-map.md")))...)
	for _, p := range problems {
		t.Error(p)
	}

	planted, err := corpus.ParseMarkdown([]byte(`---
id: test-planted
status: implemented
---

<!-- case: TC1 -->
## TC1

<!-- field: integration_target -->
### Integration target

mage integration:nowhere

<!-- field: go_test -->
### Go test

go test ./magefiles -run '^Test(SuitesNameRealTests|NoSuchTest)$'
`))
	if err != nil {
		t.Fatal(err)
	}
	got := e.suiteProblems(planted)
	joined := strings.Join(got, "\n")
	if len(got) != 2 || !strings.Contains(joined, "no target integration:nowhere") || !strings.Contains(joined, "no test TestNoSuchTest in") {
		t.Fatalf("planted suite reported as %q", got)
	}
}
