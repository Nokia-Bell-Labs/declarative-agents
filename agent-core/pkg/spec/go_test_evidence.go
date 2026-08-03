// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import (
	"bufio"
	"encoding/json"
	"fmt"
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

// ParseGoTestInventory builds an inventory from the outputs of the profile's
// declared `go list -m`, `go list ./...`, and `go test -json -list` exec words.
// Keeping command execution in the profile makes the subprocess boundary
// visible while this package remains a deterministic schema reducer.
func ParseGoTestInventory(moduleOutput, packagesOutput, testsOutput string) (*GoTestInventory, error) {
	modulePath := strings.TrimSpace(moduleOutput)
	if modulePath == "" {
		return nil, fmt.Errorf("go list -m returned no module path")
	}
	packages := parseGoPackages(packagesOutput)
	if len(packages) == 0 {
		return nil, fmt.Errorf("go list ./... returned no packages")
	}
	byPackage, allTests, err := parseGoListTests(strings.NewReader(testsOutput))
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
	// The value parsed neither as a go test command nor as a valid bare-name
	// list. If it is nonetheless an intended Go-test reference — every
	// comma-separated member begins with the "Test" prefix — it is malformed
	// (embedded spaces, a stale global rename, etc.) and must be reported
	// rather than silently treated as descriptive prose, which would leave the
	// formal suite green while the reference proves nothing (GH-1350).
	if problem := malformedTestNameList(evidence); problem != "" {
		return problem
	}
	return "" // descriptive evidence, not a validated form
}

// malformedTestNameList reports a problem when evidence looks like an intended
// comma-separated list of bare Go test names — every member begins with "Test"
// — but at least one member is not a valid top-level test identifier. When any
// member does not begin with "Test", the value is treated as descriptive
// evidence (returns ""), keeping the validator conservative about prose.
func malformedTestNameList(evidence string) string {
	parts := strings.Split(evidence, ",")
	var malformed []string
	for _, p := range parts {
		name := strings.TrimSpace(p)
		if name == "" || !strings.HasPrefix(name, "Test") {
			return "" // not an intended test-name list; leave as prose
		}
		if !goTestNameRE.MatchString(name) {
			malformed = append(malformed, fmt.Sprintf("%q", name))
		}
	}
	if len(malformed) == 0 {
		return "" // a well-formed list is handled by bareTestNames
	}
	return fmt.Sprintf("malformed Go test name %s (not a valid top-level test "+
		"identifier); use a bare Test name, a comma-separated list of them, or a "+
		"`go test` command", strings.Join(malformed, ", "))
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
	pkgArgs, runPattern, problem := parseGoTestCommand(evidence)
	if problem != "" {
		return problem
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

// parseGoTestCommand splits a "go test <pkgs> [-run <pattern>]" evidence string
// into its package arguments and -run pattern, or reports a problem. Shared by
// the resolver, which asks whether the command names anything real, and the
// runner, which asks whether what it names passed; splitting the parse means the
// two cannot disagree about what a command says.
func parseGoTestCommand(evidence string) (pkgArgs []string, runPattern, problem string) {
	fields := strings.Fields(evidence)
	fields = fields[2:] // drop the "go test" prefix
	for i := 0; i < len(fields); i++ {
		tok := fields[i]
		switch {
		case tok == "-run":
			if i+1 >= len(fields) {
				return nil, "", "-run flag has no pattern"
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
	return pkgArgs, runPattern, ""
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
	// Each alternation branch names a separate proof, so each must match.
	// Checking only the whole pattern lets one branch match while another names
	// a test that does not exist: the command still runs something, so it exits
	// green while the missing proof goes unreported. This covers both top-level
	// alternation (GH-592) and a grouped explicit-name alternation such as
	// Test(A|B|C)$ (GH-1353), which expands to its individual named proofs.
	for _, branch := range explicitNameBranches(top) {
		re, err := regexp.Compile(branch)
		if err != nil {
			return fmt.Sprintf("invalid -run regex %q: %v", pattern, err)
		}
		if inv.matchesAny(re, pkgs) {
			continue
		}
		if branch == top {
			return fmt.Sprintf("-run %q matches no test in %s", pattern, strings.Join(pkgs, ", "))
		}
		return fmt.Sprintf("-run %q names %q, which matches no test in %s",
			pattern, branch, strings.Join(pkgs, ", "))
	}
	return ""
}

// matchesAny reports whether re matches a test in any of the scoped packages.
func (inv *GoTestInventory) matchesAny(re *regexp.Regexp, pkgs []string) bool {
	for _, pkg := range pkgs {
		for name := range inv.byPackage[pkg] {
			if re.MatchString(name) {
				return true
			}
		}
	}
	return false
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

func parseGoPackages(out string) map[string]bool {
	packages := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			packages[line] = true
		}
	}
	return packages
}

// goListEvent is the subset of the `go test -json` event shape the inventory
// reads: the emitting package and one line of list output.
type goListEvent struct {
	Action  string
	Package string
	Output  string
}

// parseGoListTests keeps only top-level test names from a go test -json -list
// stream, discarding package summaries and other output events.
func parseGoListTests(stream *strings.Reader) (map[string]map[string]bool, map[string]bool, error) {
	byPackage := map[string]map[string]bool{}
	allTests := map[string]bool{}
	scanner := bufio.NewScanner(stream)
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
	return byPackage, allTests, nil
}
