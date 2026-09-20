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

	"gopkg.in/yaml.v3"
)

// TestSpecGoTestEvidenceSelectorsMatchATest requires every `-run` selector on
// an implemented test case to match at least one test function in the packages
// the command names.
//
// `go test -run 'TestThatDoesNotExist'` exits zero. No tests run, nothing
// fails, and the evidence reports as passing while asserting nothing. That is
// the defect: a case marked implemented whose command tests nothing. The
// rel06.0 suite held one, a `cd ../magefiles` resolving to a directory that
// does not exist (GH-2375).
//
// A planned case is exempt, and that is the point rather than a concession. A
// suite records the tests a release will have; a case it marks planned names a
// test nobody has written yet, which is what planned means. srd055's
// blueprints are planned in the SRD's own implementation status, so its suite
// naming TestBlueprint functions that do not exist is the corpus being
// accurate (GH-2385).
func TestSpecGoTestEvidenceSelectorsMatchATest(t *testing.T) {
	root := repositoryRootForEvidence(t)
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
			for _, evidence := range implementedGoTestCommands(t, entry.Name(), data) {
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
					t.Errorf("%s: evidence %q: %v", entry.Name(), evidence, err)
					continue
				}
				if !matchesAny(pattern, names) {
					t.Errorf("%s: formal evidence %q matches no test function in %s; "+
						"go test -run exits zero when nothing matches, so this evidence asserts nothing",
						entry.Name(), evidence, strings.Join(packages, " "))
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no go_test selectors were checked; the guard is a no-op")
	}
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

// implementedGoTestCommands returns the go test commands of every test case
// the suite does not mark planned. A case with no status is checked: silence
// is not a claim that the test is unwritten.
func implementedGoTestCommands(t *testing.T, name string, data []byte) []string {
	t.Helper()
	var suite struct {
		TestCases []struct {
			Status string `yaml:"status"`
			GoTest string `yaml:"go_test"`
		} `yaml:"test_cases"`
	}
	if err := yaml.Unmarshal(data, &suite); err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	var commands []string
	for _, testCase := range suite.TestCases {
		if strings.EqualFold(testCase.Status, "planned") {
			continue
		}
		if strings.Contains(testCase.GoTest, "go test ") {
			commands = append(commands, strings.TrimSpace(testCase.GoTest))
		}
	}
	return commands
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

// A planned case names a test nobody has written yet, and that is what planned
// means; an implemented one claims the command proves something. The guard has
// to tell them apart, or it either excuses a real defect or reports a corpus
// that is telling the truth (GH-2385).
func TestPlannedCasesAreExemptAndImplementedOnesAreNot(t *testing.T) {
	t.Parallel()
	suite := []byte(`
test_cases:
  - name: written
    status: implemented
    go_test: go test ./internal/load -run 'TestReal'
  - name: not written yet
    status: planned
    go_test: go test ./internal/load -run 'TestNotWrittenYet'
  - name: status omitted
    go_test: go test ./internal/load -run 'TestUnstated'
  - name: not a go test
    status: implemented
    go_test: mage audit
`)
	commands := implementedGoTestCommands(t, "fixture.yaml", suite)
	joined := strings.Join(commands, "\n")
	if strings.Contains(joined, "TestNotWrittenYet") {
		t.Error("a planned case was checked; a planned test is one nobody has written")
	}
	if !strings.Contains(joined, "TestReal") {
		t.Error("an implemented case was skipped")
	}
	if !strings.Contains(joined, "TestUnstated") {
		t.Error("a case with no status was skipped; silence is not a claim that the test is unwritten")
	}
	if strings.Contains(joined, "mage audit") {
		t.Error("a mage command was collected as go test evidence")
	}
	if len(commands) != 2 {
		t.Errorf("collected %d commands, want 2: %v", len(commands), commands)
	}
}
