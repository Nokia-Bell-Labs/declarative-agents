// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strings"
)

// goTestEvidenceCheck is the check identifier stamped on findings this validator
// raises, so consumers can filter go_test evidence problems from other spec
// findings.
const goTestEvidenceCheck = "go-test-evidence"

// goTestNameRE matches a bare top-level Go test name: "Test" followed by
// identifier runes and nothing else. It anchors both ends so a bare-name
// evidence string can never match a longer test name by prefix.
var goTestNameRE = regexp.MustCompile(`^Test[A-Za-z0-9_]*$`)

// GoTestInventory is the set of top-level Go tests in a module, indexed for the
// two lookups the evidence validator needs: exact membership across the whole
// module (bare-name evidence) and per-package membership (package-scoped -run
// evidence). packages holds every resolvable import path, including those with
// no test files, so package-only evidence resolves without matching a test name.
type GoTestInventory struct {
	modulePath string
	packages   map[string]bool            // import path -> exists
	byPackage  map[string]map[string]bool // import path -> test name -> exists
	allTests   map[string]bool            // union of every top-level test name
}

// BuildGoTestInventory inventories the module rooted at moduleDir. It resolves
// the module path and package set with `go list` and lists top-level tests with
// `go test -json -list '^Test' ./...`, running each command once and executing
// no test. `-list` compiles the test binaries but runs nothing, so a command
// that names a nonexistent test still leaves a gap the validator can catch.
func BuildGoTestInventory(moduleDir string) (*GoTestInventory, error) {
	modulePath, err := goModulePath(moduleDir)
	if err != nil {
		return nil, err
	}
	packages, err := goListPackages(moduleDir)
	if err != nil {
		return nil, err
	}
	byPackage, allTests, err := goListTests(moduleDir)
	if err != nil {
		return nil, err
	}
	return &GoTestInventory{
		modulePath: modulePath,
		packages:   packages,
		byPackage:  byPackage,
		allTests:   allTests,
	}, nil
}

// AuditGoTestEvidence builds the Go test inventory for the module at rootDir,
// parses its formal test suites, and validates every go_test evidence string.
// It is the entry point mage audit invokes.
func AuditGoTestEvidence(rootDir string) ([]Finding, error) {
	inv, err := BuildGoTestInventory(rootDir)
	if err != nil {
		return nil, err
	}
	suites, err := discoverAndParseTestSuites(rootDir)
	if err != nil {
		return nil, err
	}
	return ValidateGoTestEvidence(inv, suites), nil
}

// ValidateGoTestEvidence checks the go_test evidence of every test case in
// suites against inv and returns one error-level Finding per executable evidence
// string that cannot be resolved. Only bare names, comma-separated names, and
// "go test ... [-run ...]" commands are validated; Mage, descriptive, and
// shell-pipeline evidence is skipped. Findings are sorted by suite then case so
// the report is deterministic.
func ValidateGoTestEvidence(inv *GoTestInventory, suites map[string]TestSuite) []Finding {
	suiteIDs := make([]string, 0, len(suites))
	for id := range suites {
		suiteIDs = append(suiteIDs, id)
	}
	sort.Strings(suiteIDs)

	var findings []Finding
	for _, id := range suiteIDs {
		suite := suites[id]
		for _, tc := range suite.TestCases {
			problem := inv.checkEvidence(tc.GoTest)
			if problem == "" {
				continue
			}
			findings = append(findings, Finding{
				Check:   goTestEvidenceCheck,
				Level:   "error",
				SuiteID: suite.ID,
				Message: fmt.Sprintf("test case %q go_test %q: %s",
					tc.Name, strings.TrimSpace(tc.GoTest), problem),
			})
		}
	}
	return findings
}

// checkEvidence classifies one evidence string and validates the executable
// forms. It returns "" when the evidence resolves or is intentionally skipped,
// or a human-readable problem otherwise.
func (inv *GoTestInventory) checkEvidence(raw string) string {
	evidence := strings.TrimSpace(raw)
	if evidence == "" {
		return "" // no evidence to validate
	}
	// A go-test command is validated directly. This is checked first because its
	// -run regex may contain '|', which the shell-pipeline skip below would
	// otherwise misread as a pipe.
	if isGoTestCommand(evidence) {
		return inv.checkGoTestCommand(evidence)
	}
	if skipGoTestEvidence(evidence) {
		return ""
	}
	if names, ok := bareTestNames(evidence); ok {
		return inv.checkBareNames(names)
	}
	return "" // descriptive evidence, not a validated form
}

// skipGoTestEvidence reports whether evidence is Mage or a shell pipeline, which
// the validator does not execute or parse. The inventory covers a single module,
// so a cross-module `cd other && go test ...` cannot be resolved against it, and
// R4 forbids running arbitrary shell.
func skipGoTestEvidence(evidence string) bool {
	if evidence == "mage" || strings.HasPrefix(evidence, "mage ") {
		return true
	}
	if strings.HasPrefix(evidence, "cd ") {
		return true
	}
	return strings.Contains(evidence, "&&") || strings.ContainsAny(evidence, "|;")
}

func isGoTestCommand(evidence string) bool {
	return evidence == "go test" || strings.HasPrefix(evidence, "go test ")
}

// bareTestNames splits evidence as a comma-separated list of bare test names and
// reports whether every member is a valid top-level test name. A single name is
// the one-element case.
func bareTestNames(evidence string) ([]string, bool) {
	parts := strings.Split(evidence, ",")
	names := make([]string, 0, len(parts))
	for _, p := range parts {
		name := strings.TrimSpace(p)
		if !goTestNameRE.MatchString(name) {
			return nil, false
		}
		names = append(names, name)
	}
	return names, len(names) > 0
}

// checkBareNames requires each name to be present in the module-wide test set by
// exact match, so a bare name cannot be satisfied by a longer test it prefixes.
func (inv *GoTestInventory) checkBareNames(names []string) string {
	var missing []string
	for _, name := range names {
		if !inv.allTests[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Sprintf("no Go test named %s", strings.Join(missing, ", "))
	}
	return ""
}

// checkGoTestCommand validates a "go test <pkgs> [-run <pattern>]" evidence
// string: every package pattern must resolve, and any -run pattern must match at
// least one test within the resolved packages. A package-only command is valid
// once its packages resolve.
func (inv *GoTestInventory) checkGoTestCommand(evidence string) string {
	fields := strings.Fields(evidence)
	fields = fields[2:] // drop the "go test" prefix

	var pkgArgs []string
	var runPattern string
	for i := 0; i < len(fields); i++ {
		tok := fields[i]
		switch {
		case tok == "-run":
			if i+1 >= len(fields) {
				return "-run flag has no pattern"
			}
			runPattern = stripQuotes(fields[i+1])
			i++
		case strings.HasPrefix(tok, "-run="):
			runPattern = stripQuotes(strings.TrimPrefix(tok, "-run="))
		case strings.HasPrefix(tok, "-"):
			// Other flags (e.g. -count, -tags) do not affect resolution.
		default:
			pkgArgs = append(pkgArgs, tok)
		}
	}
	if len(pkgArgs) == 0 {
		pkgArgs = []string{"./..."}
	}

	pkgs, missing := inv.resolvePackages(pkgArgs)
	if len(missing) > 0 {
		return fmt.Sprintf("unknown package %s", strings.Join(missing, ", "))
	}
	if runPattern == "" {
		return "" // package-only command: packages resolved, nothing to match
	}
	return inv.checkRunPattern(runPattern, pkgs)
}

// resolvePackages maps `go test` package arguments to import paths and collects
// any that do not resolve. "./..." and "<pkg>/..." expand against the inventory
// package set.
func (inv *GoTestInventory) resolvePackages(pkgArgs []string) (pkgs, missing []string) {
	seen := map[string]bool{}
	add := func(p string) {
		if !seen[p] {
			seen[p] = true
			pkgs = append(pkgs, p)
		}
	}
	for _, arg := range pkgArgs {
		switch {
		case arg == "./...":
			for p := range inv.packages {
				add(p)
			}
		case strings.HasSuffix(arg, "/..."):
			prefix := inv.importPath(strings.TrimSuffix(arg, "/..."))
			matched := false
			for p := range inv.packages {
				if p == prefix || strings.HasPrefix(p, prefix+"/") {
					add(p)
					matched = true
				}
			}
			if !matched {
				missing = append(missing, arg)
			}
		default:
			imp := inv.importPath(arg)
			if inv.packages[imp] {
				add(imp)
			} else {
				missing = append(missing, arg)
			}
		}
	}
	sort.Strings(pkgs)
	sort.Strings(missing)
	return pkgs, missing
}

// importPath turns a relative `go test` package argument into a full import path
// under the inventory's module.
func (inv *GoTestInventory) importPath(pkgArg string) string {
	rel := strings.TrimPrefix(pkgArg, "./")
	rel = strings.Trim(rel, "/")
	if rel == "" || rel == "." {
		return inv.modulePath
	}
	return inv.modulePath + "/" + rel
}

// checkRunPattern compiles pattern the way `go test -run` does — the top-level
// segment (before any '/') as an unanchored RE2 — and requires it to match at
// least one test in the scoped packages. This catches a pattern that matches
// nothing even though `go test -run <pattern>` exits zero when no test runs.
func (inv *GoTestInventory) checkRunPattern(pattern string, pkgs []string) string {
	top := pattern
	if idx := strings.IndexByte(pattern, '/'); idx >= 0 {
		top = pattern[:idx]
	}
	re, err := regexp.Compile(top)
	if err != nil {
		return fmt.Sprintf("invalid -run regex %q: %v", pattern, err)
	}
	for _, pkg := range pkgs {
		for name := range inv.byPackage[pkg] {
			if re.MatchString(name) {
				return "" // matched within the scoped packages
			}
		}
	}
	return fmt.Sprintf("-run %q matches no test in %s", pattern, strings.Join(pkgs, ", "))
}

// stripQuotes removes a single matching pair of surrounding single or double
// quotes, as a shell would when passing a -run pattern to go test.
func stripQuotes(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		first, last := s[0], s[len(s)-1]
		if (first == '\'' && last == '\'') || (first == '"' && last == '"') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// goModulePath returns the module path reported by `go list -m` in dir.
func goModulePath(dir string) (string, error) {
	out, err := runGoCommand(dir, "list", "-m")
	if err != nil {
		return "", err
	}
	path := strings.TrimSpace(out)
	if path == "" {
		return "", fmt.Errorf("go list -m in %s returned no module path", dir)
	}
	return path, nil
}

// goListPackages returns the set of import paths under dir from `go list ./...`.
func goListPackages(dir string) (map[string]bool, error) {
	out, err := runGoCommand(dir, "list", "./...")
	if err != nil {
		return nil, err
	}
	packages := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			packages[line] = true
		}
	}
	return packages, nil
}

// goListEvent is the subset of the `go test -json` event shape the inventory
// reads: the emitting package and one line of list output.
type goListEvent struct {
	Action  string
	Package string
	Output  string
}

// goListTests lists top-level tests per package via `go test -json -list`. It
// keeps only output lines that are valid test names, discarding the per-package
// summary lines ("ok  pkg", "?  pkg [no test files]").
func goListTests(dir string) (map[string]map[string]bool, map[string]bool, error) {
	cmd := exec.Command("go", "test", "-json", "-list", "^Test", "./...")
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	byPackage := map[string]map[string]bool{}
	allTests := map[string]bool{}
	scanner := bufio.NewScanner(&stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var ev goListEvent
		if err := json.Unmarshal(scanner.Bytes(), &ev); err != nil {
			continue
		}
		if ev.Action != "output" || ev.Package == "" {
			continue
		}
		name := strings.TrimSpace(ev.Output)
		if !goTestNameRE.MatchString(name) {
			continue
		}
		if byPackage[ev.Package] == nil {
			byPackage[ev.Package] = map[string]bool{}
		}
		byPackage[ev.Package][name] = true
		allTests[name] = true
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("scan go test -list output: %w", err)
	}
	if runErr != nil && len(allTests) == 0 {
		return nil, nil, fmt.Errorf("go test -list in %s: %w\n%s", dir, runErr, stderr.String())
	}
	return byPackage, allTests, nil
}

// runGoCommand runs `go <args>` in dir and returns stdout, wrapping any error
// with the captured stderr.
func runGoCommand(dir string, args ...string) (string, error) {
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("go %s in %s: %w\n%s", strings.Join(args, " "), dir, err, stderr.String())
	}
	return stdout.String(), nil
}
