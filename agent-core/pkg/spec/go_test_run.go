// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
)

// goTestRunCheck is the check identifier stamped on findings this validator
// raises, so a consumer can tell a claim that did not pass from a claim that did
// not resolve.
const goTestRunCheck = "go-test-evidence-run"

// evidenceRunEnv marks the `go test` child this runner spawns. The runner shells
// out to `go test ./...` over its own module, so calling it from a test *in* that
// module recurses: the child runs the calling test, which spawns another child.
// The guard turns that into an immediate, legible error instead of a hang. Unit
// tests inject a runner through runGoTestEvidenceWith and never reach here.
const evidenceRunEnv = "AGENT_CORE_EVIDENCE_RUN"

// failureOutputLines caps the per-test output carried into a finding. Enough to
// see the assertion that failed without pasting a package's whole log into an
// audit report.
const failureOutputLines = 20

// testRef identifies one top-level Go test by the package that defines it. The
// package matters: two packages may define the same test name, and a claim
// scoped to one of them is not answered by the other passing.
type testRef struct {
	pkg  string
	name string
}

// evidenceClaim is one test case's assertion that a set of tests proves it.
type evidenceClaim struct {
	suiteID  string
	caseName string
	evidence string
	tests    []testRef
}

// RunGoTestEvidence runs this module's tests once and reports every test case
// whose go_test evidence did not actually pass.
//
// This is the question resolution cannot answer. AuditGoTestEvidence proves the
// named test exists, which catches a renamed, deleted, or zero-match proof
// command -- but `go test -json -list` compiles the test binaries and executes
// none of them, so a suite could name a test that fails and the audit stayed
// green. That is not hypothetical: it is how a chatbot-mesh case claimed
// evidence for a test that had never passed since the day it was written
// (GH-701, GH-713, GH-717).
//
// The whole module runs once and every claim is matched against the results,
// rather than invoking `go test` per claim. Most evidence here is bare test
// names, which scope to the whole module, so per-claim invocation would re-run
// every package once per claim. One pass also sees a test that was *skipped*,
// which neither resolution nor a per-claim `-run` invocation can distinguish
// from success: `go test -run X` exits zero whether X passed or skipped.
func RunGoTestEvidence(rootDir string) ([]Finding, error) {
	if os.Getenv(evidenceRunEnv) != "" {
		return nil, fmt.Errorf(
			"RunGoTestEvidence called from inside its own `go test` run; it would recurse. "+
				"Run it from an audit target or the agent binary, not from a test in the module under test (%s set)",
			evidenceRunEnv)
	}
	return runGoTestEvidenceWith(rootDir, runModuleTests)
}

// moduleTestRunner runs a module's tests and returns each top-level test's
// outcome. Injected so the runner's own tests never shell out to a real module.
type moduleTestRunner func(dir string) (map[testRef]testResult, error)

func runGoTestEvidenceWith(rootDir string, run moduleTestRunner) ([]Finding, error) {
	inv, err := BuildGoTestInventory(rootDir)
	if err != nil {
		return nil, err
	}
	suites, err := discoverAndParseTestSuites(rootDir)
	if err != nil {
		return nil, err
	}
	claims, findings := collectEvidenceClaims(inv, suites)
	if len(findings) > 0 {
		// A claim that cannot be read is reported before anything runs: a proof
		// command nobody can parse is not a proof, and skipping it would recreate
		// the silent pass this validator exists to close.
		return findings, nil
	}
	if len(claims) == 0 {
		return nil, fmt.Errorf("no runnable go_test evidence found under %s; the suites or their layout changed", rootDir)
	}
	results, err := run(rootDir)
	if err != nil {
		return nil, err
	}
	return evaluateClaims(claims, results), nil
}

// collectEvidenceClaims turns every live claim into the concrete set of tests it
// rests on, in suite then case order so a report is deterministic.
//
// A case naming evidence is a claim whatever its status, except "planned". The
// status field is optional in the test_suite format rule and most cases in this
// corpus omit it, so treating an absent status as "not a claim" would leave the
// majority of the evidence unverified -- the same silent gap by another route.
func collectEvidenceClaims(inv *GoTestInventory, suites map[string]TestSuite) ([]evidenceClaim, []Finding) {
	suiteIDs := make([]string, 0, len(suites))
	for id := range suites {
		suiteIDs = append(suiteIDs, id)
	}
	sort.Strings(suiteIDs)

	var claims []evidenceClaim
	var findings []Finding
	for _, id := range suiteIDs {
		suite := suites[id]
		for _, tc := range suite.TestCases {
			if withheldClaim(tc.Status) {
				continue
			}
			tests, problem := inv.claimedTests(tc.GoTest)
			if problem != "" {
				findings = append(findings, Finding{
					Check:   goTestRunCheck,
					Level:   "error",
					SuiteID: suite.ID,
					Message: fmt.Sprintf("test case %q go_test %q: %s",
						tc.Name, strings.TrimSpace(tc.GoTest), problem),
				})
				continue
			}
			if len(tests) == 0 {
				continue // mage, prose, or cross-module evidence: not run here
			}
			claims = append(claims, evidenceClaim{
				suiteID: suite.ID, caseName: tc.Name,
				evidence: strings.TrimSpace(tc.GoTest), tests: tests,
			})
		}
	}
	return claims, findings
}

// withheldClaim reports whether a case's status withholds its evidence claim.
// Only a planned case does: it names the test it intends to rest on before that
// test has to hold.
func withheldClaim(status string) bool {
	return strings.EqualFold(strings.TrimSpace(status), "planned")
}

// claimedTests expands one evidence string into the tests it claims, or reports
// a problem. It returns no tests and no problem for evidence this validator does
// not execute -- a Mage target, a shell pipeline, a prose description -- which
// the resolver classifies the same way.
func (inv *GoTestInventory) claimedTests(raw string) ([]testRef, string) {
	evidence := strings.TrimSpace(raw)
	if evidence == "" {
		return nil, ""
	}
	// Checked before the pipeline skip, whose '|' would otherwise misread a -run
	// alternation as a shell pipe.
	if isGoTestCommand(evidence) {
		return inv.commandTests(evidence)
	}
	if skipGoTestEvidence(evidence) {
		return nil, ""
	}
	if names, ok := bareTestNames(evidence); ok {
		return inv.bareNameTests(names)
	}
	return nil, "" // descriptive evidence, not a runnable form
}

// bareNameTests resolves bare test names to every package defining them. A name
// defined in two packages claims both: the evidence names the test, not one
// copy of it.
func (inv *GoTestInventory) bareNameTests(names []string) ([]testRef, string) {
	var refs []testRef
	var missing []string
	for _, name := range names {
		found := false
		for pkg, tests := range inv.byPackage {
			if tests[name] {
				refs = append(refs, testRef{pkg: pkg, name: name})
				found = true
			}
		}
		if !found {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Sprintf("no Go test named %s", strings.Join(missing, ", "))
	}
	sortTestRefs(refs)
	return refs, ""
}

// commandTests expands a "go test <pkgs> [-run <pattern>]" command into the
// tests it would run. A command with no -run claims every test in its packages,
// which is what running it would prove.
func (inv *GoTestInventory) commandTests(evidence string) ([]testRef, string) {
	pkgArgs, runPattern, problem := parseGoTestCommand(evidence)
	if problem != "" {
		return nil, problem
	}
	pkgs, missing := inv.resolvePackages(pkgArgs)
	if len(missing) > 0 {
		return nil, fmt.Sprintf("unknown package %s", strings.Join(missing, ", "))
	}

	var re *regexp.Regexp
	if runPattern != "" {
		top := runPattern
		if idx := strings.IndexByte(runPattern, '/'); idx >= 0 {
			top = runPattern[:idx]
		}
		compiled, err := regexp.Compile(top)
		if err != nil {
			return nil, fmt.Sprintf("invalid -run regex %q: %v", runPattern, err)
		}
		re = compiled
	}

	var refs []testRef
	for _, pkg := range pkgs {
		for name := range inv.byPackage[pkg] {
			if re == nil || re.MatchString(name) {
				refs = append(refs, testRef{pkg: pkg, name: name})
			}
		}
	}
	if len(refs) == 0 {
		return nil, fmt.Sprintf("matches no test in %s", strings.Join(pkgs, ", "))
	}
	sortTestRefs(refs)
	return refs, ""
}

func sortTestRefs(refs []testRef) {
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].pkg != refs[j].pkg {
			return refs[i].pkg < refs[j].pkg
		}
		return refs[i].name < refs[j].name
	})
}

// testResult is one top-level test's outcome and, when it failed, the tail of
// its output.
type testResult struct {
	action string // pass | fail | skip
	output []string
}

// goTestEvent is the subset of `go test -json` this runner reads.
type goTestEvent struct {
	Action  string
	Package string
	Test    string
	Output  string
}

// runModuleTests runs the module's tests once and returns each top-level test's
// outcome. -count=1 defeats the test cache: a cached pass is a claim about a
// previous tree, and the audit is asking about this one.
//
// A non-zero exit is expected whenever a test fails, so it is not an error here;
// the failures are reported per claim. An exit with no results at all is a real
// error -- a build failure, not a test failure -- and surfaces as one.
func runModuleTests(dir string) (map[testRef]testResult, error) {
	cmd := exec.Command("go", "test", "-json", "-count=1", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), evidenceRunEnv+"=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	results, err := scanTestEvents(&stdout)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("go test in %s produced no test results: %w\n%s", dir, runErr, stderr.String())
	}
	return results, nil
}

// scanTestEvents folds a `go test -json` stream into per-test outcomes. Subtests
// report as "Parent/child" and are ignored: the claim is on the parent, whose
// own pass or fail already accounts for them.
func scanTestEvents(stream *bytes.Buffer) (map[testRef]testResult, error) {
	results := map[testRef]testResult{}
	scanner := bufio.NewScanner(stream)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var ev goTestEvent
		if err := json.Unmarshal(scanner.Bytes(), &ev); err != nil {
			continue
		}
		if ev.Test == "" || strings.Contains(ev.Test, "/") {
			continue
		}
		ref := testRef{pkg: ev.Package, name: ev.Test}
		switch ev.Action {
		case "output":
			results[ref] = appendOutput(results[ref], ev.Output)
		case "pass", "fail", "skip":
			result := results[ref]
			result.action = ev.Action
			results[ref] = result
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan go test -json output: %w", err)
	}
	return results, nil
}

// appendOutput keeps the tail of a test's output, so a finding shows the
// assertion that failed rather than the whole log.
func appendOutput(result testResult, line string) testResult {
	line = strings.TrimRight(line, "\n")
	if strings.TrimSpace(line) == "" {
		return result
	}
	result.output = append(result.output, line)
	if len(result.output) > failureOutputLines {
		result.output = result.output[len(result.output)-failureOutputLines:]
	}
	return result
}

// evaluateClaims reports every claim whose tests did not all pass. Each claim is
// evaluated before returning, so one broken suite does not hide the rest.
func evaluateClaims(claims []evidenceClaim, results map[testRef]testResult) []Finding {
	var findings []Finding
	for _, claim := range claims {
		var problems []string
		for _, ref := range claim.tests {
			result, ran := results[ref]
			switch {
			case !ran || result.action == "":
				// Named, resolvable, and never executed: a build tag, a filtered
				// package, or a test the run never reached. It proves nothing.
				problems = append(problems, fmt.Sprintf("%s did not run (%s)", ref.name, ref.pkg))
			case result.action == "skip":
				// A skipped test proves nothing either, and reads as a pass to any
				// check that only watches the exit code.
				problems = append(problems, fmt.Sprintf("%s was skipped (%s)%s",
					ref.name, ref.pkg, skipReason(result.output)))
			case result.action == "fail":
				problems = append(problems, fmt.Sprintf("%s failed (%s)\n%s",
					ref.name, ref.pkg, indentOutput(result.output)))
			}
		}
		if len(problems) == 0 {
			continue
		}
		findings = append(findings, Finding{
			Check:   goTestRunCheck,
			Level:   "error",
			SuiteID: claim.suiteID,
			Message: fmt.Sprintf("test case %q go_test %q did not pass:\n    %s",
				claim.caseName, claim.evidence, strings.Join(problems, "\n    ")),
		})
	}
	return findings
}

// skipReason pulls the reason off a skipped test's output so the report says
// what to install rather than what to debug.
func skipReason(output []string) string {
	for _, line := range output {
		trimmed := strings.TrimSpace(line)
		if idx := strings.Index(trimmed, ": "); idx >= 0 && strings.Contains(trimmed, "_test.go:") {
			return ": " + strings.TrimSpace(trimmed[idx+2:])
		}
	}
	return ""
}

func indentOutput(output []string) string {
	if len(output) == 0 {
		return "      (no output captured)"
	}
	lines := make([]string, 0, len(output))
	for _, line := range output {
		lines = append(lines, "      "+strings.TrimSpace(line))
	}
	return strings.Join(lines, "\n")
}
