// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// juristProfile is the wrapper an operator ships — agents/jurist/profile.yaml —
// run directly, not a synthesized reconstruction. The shipped profile's
// /opt/agent-core tool_config_dirs and its load_corpus tool declaration (bound to
// the builtin spec-corpus charter) are resolved onto the checkout by --core-root
// (spec.SetAgentCoreInstallRoot). load_corpus reads the specification docs from
// the run directory, so --directory points at a fixture whose docs/ tree the
// profile validates.
var juristProfile = filepath.Join("agents", "jurist", "profile.yaml")

// juristCleanFixture validates clean: its findings are all warnings, so the
// builtin spec-corpus charter reaches the Passed terminal.
var juristCleanFixture = ProfilePath(filepath.Join("testdata", "integration", "jurist-charter-demo"))

// juristFailingFixture is the clean fixture plus one requirement item left
// uncovered by any acceptance criterion, which raises an error-level
// uncovered-req-item finding and drives the run to the Failed terminal.
var juristFailingFixture = ProfilePath(filepath.Join("testdata", "integration", "jurist-charter-demo-failing"))

// TestJuristConformance runs the shipped jurist profile through the agent CLI
// and asserts the srd005-jurist deterministic contract from the trace and the
// formatted report. Jurist is the pilot family that proves the harness end to
// end because it needs no model, no child agent, and no server: the pipeline is
// deterministic, LLM-free, network-free, and read-only.
//
// The subtests are the contract, one requirement group each:
//
//   - CleanCorpusPasses  — R2/R3.1/R3.3: the tool pipeline is visible and a clean
//     corpus reaches the Passed terminal.
//   - FailingCorpusFails — R3.1/R3.2/R3.3: a corpus with one error-level violation
//     reaches the Failed terminal and the report carries provenance-rich findings.
//   - Deterministic      — R1.2: identical input yields identical findings and
//     terminal across repeated runs.
//   - ReadOnly           — R1.2: the run mutates no file in the target directory.
//
// Both outcomes are asserted by separate fixtures rather than one run accepting
// either terminal. Mirrors the profile wiring of magefiles/validation.go
// validateJuristCharterDemo.
func TestJuristConformance(t *testing.T) {
	RequireCoreRoot(t)

	t.Run("CleanCorpusPasses", func(t *testing.T) {
		result := runJurist(t, juristCleanFixture)

		// srd005-jurist R1: the deterministic pipeline runs to a clean CLI exit
		// with a single root span and no error-status spans.
		result.RequireExit(t, 0)
		result.RootRequired(t)
		result.RequireNoErrorSpans(t)

		// srd005-jurist R2: the jurist tool pipeline is visible as tool spans.
		result.RequireToolSpans(t, "load_corpus", "validate_specs", "format_report")

		// srd005-jurist R3.1/R3.3: a corpus that carries no error-level violation
		// reaches the Passed terminal with a formatted report.
		result.RequireTerminalState(t, "Passed")
		if got := errorFindings(parseFindings(t, result.Output)); len(got) != 0 {
			t.Fatalf("clean corpus produced error-level findings: %v", got)
		}
	})

	t.Run("FailingCorpusFails", func(t *testing.T) {
		result := runJurist(t, juristFailingFixture)

		// A failed validation is a domain outcome, not an infrastructure error:
		// the machine reached its Failed terminal, so the CLI exits 2 (a clean
		// domain failure) rather than 1 (the binary could not run), and the
		// trace still has one root span and no error-status spans
		// (srd018-cli-flag-contract R6).
		result.RequireExit(t, 2)
		result.RootRequired(t)
		result.RequireNoErrorSpans(t)
		result.RequireToolSpans(t, "load_corpus", "validate_specs", "format_report")

		// srd005-jurist R3.3: a corpus with an error-level violation reaches the
		// Failed terminal.
		result.RequireTerminalState(t, "Failed")

		// srd005-jurist R3.2: findings identify the suite, check, severity, message,
		// and any available file/line provenance, so consumers can act on failures
		// without reading Go code.
		findings := parseFindings(t, result.Output)
		errs := errorFindings(findings)
		if len(errs) == 0 {
			t.Fatalf("failing corpus produced no error-level findings; report:\n%s", result.Output)
		}
		for _, f := range findings {
			f.requireProvenance(t)
		}
		if !hasFinding(errs, "builtin-spec-corpus", "uncovered-req-item", "srd001-demo:R1.2") {
			t.Fatalf("missing the expected uncovered-req-item finding; got %v", errs)
		}
	})

	t.Run("Deterministic", func(t *testing.T) {
		// R1.2: repeat the identical input (same target dir + suite files) and
		// assert identical findings and terminal across runs. The failing fixture
		// carries both an error and warnings, so it exercises the full report.
		first := runJurist(t, juristFailingFixture)
		second := runJurist(t, juristFailingFixture)

		firstState, _ := first.TerminalOutcome(t)
		secondState, _ := second.TerminalOutcome(t)
		if firstState != secondState {
			t.Fatalf("terminal state not deterministic: %q then %q", firstState, secondState)
		}

		firstReport := findingLines(first.Output)
		secondReport := findingLines(second.Output)
		if strings.Join(firstReport, "\n") != strings.Join(secondReport, "\n") {
			t.Fatalf("findings not deterministic across runs:\nfirst:\n%s\nsecond:\n%s",
				strings.Join(firstReport, "\n"), strings.Join(secondReport, "\n"))
		}
		if len(firstReport) == 0 {
			t.Fatalf("determinism check saw an empty report; output:\n%s", first.Output)
		}
	})

	t.Run("ReadOnly", func(t *testing.T) {
		// R1.2: the charter pipeline is read-only. Snapshot the target directory
		// before and after a run and assert no file was created, deleted, or
		// modified. The run is network-free by the same token: it reaches an
		// identical terminal offline with no fixture mutation, so no external
		// state is consulted or written.
		before := snapshotTree(t, juristCleanFixture)
		runJurist(t, juristCleanFixture)
		after := snapshotTree(t, juristCleanFixture)

		requireSameTree(t, before, after)
	})
}

// runJurist invokes the shipped jurist profile over dir and returns the trace
// plus CLI exit state. It skips when the sibling agent-core checkout is absent.
func runJurist(t *testing.T, dir string) RunResult {
	t.Helper()
	return Run(t, RunConfig{Profile: juristProfile, Directory: dir})
}

// finding is one parsed line of the jurist report: the provenance a consumer
// reads to act on a violation without reading Go code.
type finding struct {
	Level   string // "error" or "warning"
	Suite   string // charter suite identifier
	Check   string // suite-local check identifier
	Kind    string // charter check kind
	File    string // target-relative path, when the check reports one
	Line    int    // 1-based line, when the check reports one
	Message string
}

// requireProvenance asserts the always-present provenance fields are populated.
// File and line are asserted only when the finding reports them ("any available
// file and line provenance").
func (f finding) requireProvenance(t *testing.T) {
	t.Helper()
	if f.Suite == "" || f.Check == "" || f.Level == "" || f.Message == "" {
		t.Fatalf("finding missing provenance: %+v", f)
	}
	if f.Line != 0 && f.File == "" {
		t.Fatalf("finding reports a line without a file: %+v", f)
	}
}

var (
	// findingHeaderRE matches "[error] builtin-spec-corpus/uncovered-req-item (spec_corpus):".
	findingHeaderRE = regexp.MustCompile(`^\[(error|warning)\] ([^/]+)/(\S+) \(([^)]+)\):$`)
	// findingFileRE matches the "file:line: " or "file: " provenance prefix on a bullet.
	findingFileRE = regexp.MustCompile(`^(\S+?)(?::(\d+))?: (.*)$`)
)

// parseFindings reads the formatted report out of the CLI output. The report is
// a run of "[level] suite/check (kind):" headers, each followed by "  - <bullet>"
// lines whose bullet optionally carries "file[:line]: " provenance.
func parseFindings(t *testing.T, output string) []finding {
	t.Helper()
	var out []finding
	var header []string // level, suite, check, kind
	for _, line := range strings.Split(output, "\n") {
		if m := findingHeaderRE.FindStringSubmatch(line); m != nil {
			header = m[1:]
			continue
		}
		bullet, ok := strings.CutPrefix(line, "  - ")
		if !ok || header == nil {
			continue
		}
		f := finding{Level: header[0], Suite: header[1], Check: header[2], Kind: header[3], Message: bullet}
		if m := findingFileRE.FindStringSubmatch(bullet); m != nil {
			f.File = m[1]
			f.Message = m[3]
			if m[2] != "" {
				f.Line, _ = strconv.Atoi(m[2])
			}
		}
		out = append(out, f)
	}
	if len(out) == 0 {
		t.Fatalf("no findings parsed from report; output:\n%s", output)
	}
	return out
}

// findingLines returns the report's header and bullet lines verbatim, the
// deterministic slice compared across runs (log lines carry timestamps and are
// excluded).
func findingLines(output string) []string {
	var lines []string
	for _, line := range strings.Split(output, "\n") {
		if findingHeaderRE.MatchString(line) || strings.HasPrefix(line, "  - ") {
			lines = append(lines, line)
		}
	}
	return lines
}

func errorFindings(findings []finding) []finding {
	var out []finding
	for _, f := range findings {
		if f.Level == "error" {
			out = append(out, f)
		}
	}
	return out
}

// hasFinding reports whether some finding matches the suite and check and whose
// message contains want.
func hasFinding(findings []finding, suite, check, want string) bool {
	for _, f := range findings {
		if f.Suite == suite && f.Check == check && strings.Contains(f.Message, want) {
			return true
		}
	}
	return false
}

// snapshotTree maps each regular file under dir to the hex sha256 of its
// contents, so a later snapshot detects any create, delete, or modify.
func snapshotTree(t *testing.T, dir string) map[string]string {
	t.Helper()
	tree := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		tree[rel] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot %s: %v", dir, err)
	}
	return tree
}

// requireSameTree fails if any file was added, removed, or changed between two
// snapshots.
func requireSameTree(t *testing.T, before, after map[string]string) {
	t.Helper()
	for rel, sum := range before {
		switch got, ok := after[rel]; {
		case !ok:
			t.Errorf("file removed by run: %s", rel)
		case got != sum:
			t.Errorf("file modified by run: %s", rel)
		}
	}
	for rel := range after {
		if _, ok := before[rel]; !ok {
			t.Errorf("file created by run: %s", rel)
		}
	}
}
