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
	"strings"
	"testing"
)

// TestSpecGoTestEvidenceSelectorsMatchATest requires every `-run` selector
// named as formal go_test evidence to match at least one test function in the
// packages the command names.
//
// `go test -run 'TestThatDoesNotExist'` exits zero. No tests run, nothing
// fails, and the evidence reports as passing while asserting nothing. Two
// entries in test-rel20.0-machine-templates.yaml were in that state and had
// been since srd055 was written: they named TestBlueprint functions that exist
// nowhere, so a requirement was recorded as covered by a command that tested
// nothing (GH-2375).
//
// This is the same defect class as GH-2333 one layer down. That guard checks a
// Mage target resolves; this one checks a Go selector does.
func TestSpecGoTestEvidenceSelectorsMatchATest(t *testing.T) {
	root := repositoryRootForEvidence(t)
	baseline, err := loadEmptySelectorBaseline(filepath.Join(root, emptySelectorBaselinePath))
	if err != nil {
		t.Fatalf("read %s: %v", emptySelectorBaselinePath, err)
	}
	seen := map[string]bool{}
	checked := 0
	for _, corpus := range []string{
		filepath.Join("agent-core", "docs", "specs", "test-suites"),
		filepath.Join("applications", "catalog", "docs", "specs", "test-suites"),
		filepath.Join("applications", "docs", "specs", "test-suites"),
	} {
		suitesDir := filepath.Join(root, corpus)
		entries, err := os.ReadDir(suitesDir)
		if err != nil {
			continue // a corpus this checkout does not carry
		}
		// A suite's relative commands resolve against its module, which is the
		// directory holding the docs tree.
		moduleRoot := filepath.Dir(filepath.Dir(filepath.Dir(suitesDir)))
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(suitesDir, entry.Name()))
			if err != nil {
				t.Fatalf("read %s: %v", entry.Name(), err)
			}
			for _, evidence := range goTestEvidenceCommands(string(data)) {
				base, packages, selector, ok := parseGoTestEvidence(evidence)
				if !ok || selector == "" {
					continue // no -run selector: the command runs a whole package
				}
				pattern, err := regexp.Compile(selector)
				if err != nil {
					t.Errorf("%s: evidence %q has an unparsable -run selector: %v",
						entry.Name(), evidence, err)
					continue
				}
				checked++
				names, err := testFunctionNames(filepath.Join(moduleRoot, base), packages)
				if err != nil {
					key := entry.Name() + " " + selector
					seen[key] = true
					if baseline[key] {
						continue
					}
					t.Errorf("%s: evidence %q: %v", entry.Name(), evidence, err)
					continue
				}
				if !matchesAny(pattern, names) {
					key := entry.Name() + " " + selector
					seen[key] = true
					if baseline[key] {
						continue
					}
					t.Errorf("%s: formal evidence %q matches no test function in %s; "+
						"go test -run exits zero when nothing matches, so this evidence asserts nothing",
						entry.Name(), evidence, strings.Join(packages, " "))
				}
			}
		}
	}
	for key := range baseline {
		if !seen[key] {
			t.Errorf("%s grandfathers %q, whose selector now resolves or no longer exists; delete the line",
				emptySelectorBaselinePath, key)
		}
	}
	if checked == 0 {
		t.Fatal("no go_test selectors were checked; the guard is a no-op")
	}
}

// emptySelectorBaselinePath lists the selectors that matched nothing when this
// guard was written. They are grandfathered so the count cannot grow, not
// excused: each line is a requirement whose formal evidence runs no test, and
// GH-2385 tracks writing the tests that would let the line be deleted.
const emptySelectorBaselinePath = "agent-core/docs/specs/legacy-empty-test-selectors.txt"

// loadEmptySelectorBaseline reads "<suite file> <selector>" lines. An absent
// file is an empty baseline.
func loadEmptySelectorBaseline(path string) (map[string]bool, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- repository-owned path
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]bool{}, nil
		}
		return nil, err
	}
	entries := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		entries[line] = true
	}
	return entries, nil
}

// matchesAny reports whether the selector matches a test name. go test splits
// a name on "/" for subtests and matches each element, so a selector naming a
// subtest matches when its first element names a real function.
func matchesAny(pattern *regexp.Regexp, names []string) bool {
	for _, name := range names {
		if pattern.MatchString(name) {
			return true
		}
	}
	return false
}

// goTestEvidenceCommands returns the inline value of every `go_test:` line
// that runs go test rather than mage.
func goTestEvidenceCommands(doc string) []string {
	var values []string
	for _, line := range strings.Split(doc, "\n") {
		trimmed := strings.TrimSpace(line)
		value, ok := strings.CutPrefix(trimmed, "go_test:")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		if strings.Contains(value, "go test ") {
			values = append(values, value)
		}
	}
	return values
}

var goTestRunSelector = regexp.MustCompile(`-run\s+'([^']*)'|-run\s+"([^"]*)"`)

// parseGoTestEvidence splits `[cd <rel> && ] go test <packages...> [-run 'x']`
// into the directory the command runs in, the packages it names, and the
// selector. It reports ok=false for anything else, including a mage command.
func parseGoTestEvidence(evidence string) (base string, packages []string, selector string, ok bool) {
	command := evidence
	base = "."
	if rest, isCd := strings.CutPrefix(evidence, "cd "); isCd {
		parts := strings.SplitN(rest, "&&", 2)
		if len(parts) != 2 {
			return "", nil, "", false
		}
		base = strings.TrimSpace(parts[0])
		command = strings.TrimSpace(parts[1])
	}
	args, isGoTest := strings.CutPrefix(command, "go test ")
	if !isGoTest {
		return "", nil, "", false
	}
	if match := goTestRunSelector.FindStringSubmatch(args); match != nil {
		selector = match[1] + match[2]
		args = args[:strings.Index(args, "-run")]
	}
	for _, field := range strings.Fields(args) {
		if strings.HasPrefix(field, "./") || field == "." {
			packages = append(packages, field)
		}
	}
	if len(packages) == 0 {
		return "", nil, "", false
	}
	return base, packages, selector, true
}

// testFunctionNames walks each named package for exported Test functions,
// following a trailing /... as go test does.
func testFunctionNames(base string, packages []string) ([]string, error) {
	var names []string
	for _, pkg := range packages {
		dir := filepath.Join(base, filepath.FromSlash(strings.TrimSuffix(pkg, "/...")))
		recursive := strings.HasSuffix(pkg, "/...")
		found, err := testFunctionsUnder(dir, recursive)
		if err != nil {
			return nil, err
		}
		names = append(names, found...)
	}
	return names, nil
}

func testFunctionsUnder(dir string, recursive bool) ([]string, error) {
	var names []string
	walk := func(path string) error {
		fset := token.NewFileSet()
		pkgs, err := parser.ParseDir(fset, path, func(fi os.FileInfo) bool {
			return strings.HasSuffix(fi.Name(), "_test.go")
		}, 0)
		if err != nil {
			return err
		}
		for _, pkg := range pkgs {
			for _, file := range pkg.Files {
				for _, decl := range file.Decls {
					fn, isFunc := decl.(*ast.FuncDecl)
					if isFunc && fn.Recv == nil && strings.HasPrefix(fn.Name.Name, "Test") {
						names = append(names, fn.Name.Name)
					}
				}
			}
		}
		return nil
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("package directory %s does not exist", dir)
	}
	if !recursive {
		return names, walk(dir)
	}
	return names, filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil || !entry.IsDir() {
			return err
		}
		name := entry.Name()
		if path != dir && (name == "testdata" || name == "node_modules" || strings.HasPrefix(name, ".")) {
			return filepath.SkipDir
		}
		return walk(path)
	})
}

// repositoryRootForEvidence walks up from this module to the directory holding
// both agent-core and applications.
func repositoryRootForEvidence(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(dir, "agent-core")); err == nil {
			if _, err := os.Stat(filepath.Join(dir, "applications")); err == nil {
				return dir
			}
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("repository root not found above the magefiles module")
	return ""
}

// The GH-2375 guard proven against the defect it describes: a selector naming
// a function that does not exist must fail, and one naming a function that
// does must pass. Without this, the guard could be silently inverted and the
// corpus would go on reporting evidence that runs nothing.
func TestEmptySelectorIsReportedAndRealSelectorIsNot(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	source := "package fixture\n\nimport \"testing\"\n\nfunc TestRealOne(t *testing.T) {}\n"
	if err := os.WriteFile(filepath.Join(dir, "fixture_test.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	names, err := testFunctionNames(dir, []string{"."})
	if err != nil {
		t.Fatal(err)
	}
	for selector, want := range map[string]bool{
		"TestRealOne":                  true,
		"TestRealOne|TestAlsoNotThere": true,
		"TestNotThere":                 false,
		"TestReal.*":                   true,
		"TestNope|TestAlsoNope":        false,
	} {
		pattern, err := regexp.Compile(selector)
		if err != nil {
			t.Fatalf("selector %q: %v", selector, err)
		}
		if got := matchesAny(pattern, names); got != want {
			t.Errorf("selector %q matched=%v, want %v", selector, got, want)
		}
	}
}

// The parser has to split the three command shapes the corpus uses, and to
// leave a command with no selector alone: `go test ./...` runs a whole package
// and asserts plenty.
func TestParseGoTestEvidenceSplitsTheCorpusShapes(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		evidence string
		base     string
		packages []string
		selector string
		ok       bool
	}{
		{"go test ./internal/load -run 'TestOne'", ".", []string{"./internal/load"}, "TestOne", true},
		{"cd ../../agent-core && go test ./internal/tools/service -run 'TestTwo|TestThree'",
			"../../agent-core", []string{"./internal/tools/service"}, "TestTwo|TestThree", true},
		{"go test ./cmd/agent ./internal/load -run 'TestFour'", ".",
			[]string{"./cmd/agent", "./internal/load"}, "TestFour", true},
		{"go test ./...", ".", []string{"./..."}, "", true},
		{"mage audit", "", nil, "", false},
	} {
		base, packages, selector, ok := parseGoTestEvidence(testCase.evidence)
		if ok != testCase.ok {
			t.Errorf("%q ok=%v, want %v", testCase.evidence, ok, testCase.ok)
			continue
		}
		if !ok {
			continue
		}
		if base != testCase.base || selector != testCase.selector ||
			strings.Join(packages, " ") != strings.Join(testCase.packages, " ") {
			t.Errorf("%q -> base=%q packages=%v selector=%q, want base=%q packages=%v selector=%q",
				testCase.evidence, base, packages, selector,
				testCase.base, testCase.packages, testCase.selector)
		}
	}
}
