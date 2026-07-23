// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// validateTestEvidence resolves every formal test suite's go_test evidence in
// this example against its real Go tests, by invoking the agent binary's
// --validate-test-evidence preflight.
//
// The resolver lives in agent-core (spec.AuditGoTestEvidence) and this example
// deliberately does not import agent-core, so the check is reached through the
// binary the audit already builds — the same way the boot smoke reuses
// --validate-config. Without it a proof command can name a test that lives in
// another module and still exit green (GH-592, GH-652).
func validateTestEvidence(run profileSmokeRunner, binary, root string) error {
	out, err := run(binary, "--validate-test-evidence", "--directory", root)
	if err == nil {
		fmt.Printf("resolved formal go_test evidence under %s\n", root)
		return nil
	}
	detail := strings.TrimSpace(string(out))
	if detail == "" {
		detail = err.Error()
	}
	return fmt.Errorf("formal go_test evidence validation failed:\n%s", detail)
}

// evidenceRunner runs one `go` invocation in dir and returns its combined
// output. Injected so the evidence run can be tested without executing tests.
type evidenceRunner func(dir string, args ...string) ([]byte, error)

func defaultEvidenceRun(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	return cmd.CombinedOutput()
}

// evidenceCommand is one runnable go_test evidence string, carried with every
// case that claims it so a failure names all of its claims. Two cases may cite
// the same command -- the hot-swap and fan-out cases both rest on the
// co-generation test -- and the command runs once, but a failure belongs to
// both claimants, not just whichever suite sorted first.
type evidenceCommand struct {
	claimants []string // "<suite>: <case name>"
	args      []string // args to `go`, e.g. {"test", "./magefiles", "-run", "^TestX$"}
}

// runTestEvidence runs the go_test command of every formal test case that claims
// to be implemented, and fails the audit when one does not pass.
//
// Resolution and execution answer different questions. The resolver
// (validateTestEvidence) proves the named test exists, which is what catches a
// proof command pointing at another module or a renamed test. It runs nothing:
// `go test -list` compiles the test binaries and executes none of them. So a
// suite could record status: implemented for a test that fails, and the audit
// stayed green -- which is exactly what happened to the LLM preload gate case,
// whose evidence had never passed since the day it was written (GH-701, GH-713).
//
// Only cases claiming implemented or done are run. A planned case names no
// evidence it has to honor yet. Integration work is named separately, in
// integration_target, and stays gated behind its own mage targets rather than
// running here.
//
// Every command is attempted before reporting, so one failing suite does not
// hide the rest.
func runTestEvidence(run evidenceRunner, root string) error {
	commands, err := implementedEvidenceCommands(root)
	if err != nil {
		return err
	}
	if len(commands) == 0 {
		return fmt.Errorf("no runnable go_test evidence found under %s; the suites or their layout changed", root)
	}
	var failures []string
	for _, command := range commands {
		out, err := run(root, command.args...)
		if err == nil {
			continue
		}
		detail := strings.TrimSpace(string(out))
		if detail == "" {
			detail = err.Error()
		}
		failures = append(failures, fmt.Sprintf("  %s\n    go %s\n%s",
			strings.Join(command.claimants, "\n  "), strings.Join(command.args, " "),
			indentLines(detail, "    ")))
	}
	if len(failures) > 0 {
		return fmt.Errorf("%d of %d go_test evidence command(s) failed, claimed by these test cases:\n%s",
			len(failures), len(commands), strings.Join(failures, "\n"))
	}
	fmt.Printf("ran %d go_test evidence commands for %d implemented test cases; all passed\n",
		len(commands), countClaimants(commands))
	return nil
}

// implementedEvidenceCommands reads the formal test suites and returns one
// runnable command per distinct go_test evidence string claimed by an
// implemented case, in suite order so a report is deterministic.
//
// Evidence that is not a go-test command -- a mage target, a prose description
// -- is skipped: the resolver already classifies those, and re-deciding the
// taxonomy here would let the two drift. Evidence that reads as a go-test
// command but does not parse is an error rather than a skip. Dropping it
// silently would recreate the failure this whole step exists to close: a case
// claiming implemented evidence that nothing ever runs.
func implementedEvidenceCommands(root string) ([]evidenceCommand, error) {
	pattern := filepath.Join(root, "docs", "specs", "test-suites", "*.yaml")
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)

	var commands []evidenceCommand
	byArgs := map[string]int{} // arg vector -> index in commands
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		var suite struct {
			ID        string `yaml:"id"`
			TestCases []struct {
				Name   string `yaml:"name"`
				Status string `yaml:"status"`
				GoTest string `yaml:"go_test"`
			} `yaml:"test_cases"`
		}
		if err := yaml.Unmarshal(data, &suite); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		for _, testCase := range suite.TestCases {
			if !claimsImplemented(testCase.Status) {
				continue
			}
			args, err := goTestArgs(testCase.GoTest)
			if err != nil {
				return nil, fmt.Errorf("%s: test case %q go_test %q: %w",
					suite.ID, testCase.Name, strings.TrimSpace(testCase.GoTest), err)
			}
			if args == nil {
				continue
			}
			claimant := fmt.Sprintf("%s: test case %q", suite.ID, testCase.Name)
			key := strings.Join(args, "\x00")
			if index, ok := byArgs[key]; ok {
				commands[index].claimants = append(commands[index].claimants, claimant)
				continue
			}
			byArgs[key] = len(commands)
			commands = append(commands, evidenceCommand{claimants: []string{claimant}, args: args})
		}
	}
	return commands, nil
}

// countClaimants totals the test cases behind a command set, which exceeds the
// command count whenever two cases rest on the same proof.
func countClaimants(commands []evidenceCommand) int {
	total := 0
	for _, command := range commands {
		total += len(command.claimants)
	}
	return total
}

// claimsImplemented reports whether a test case claims its evidence holds today.
func claimsImplemented(status string) bool {
	switch strings.TrimSpace(strings.ToLower(status)) {
	case "implemented", "done":
		return true
	default:
		return false
	}
}

// goTestArgs turns one go_test evidence string into args for `go`. It returns
// nil args and no error when the evidence is deliberately not run -- a mage
// target, a prose description -- and an error when the evidence reads as a
// go-test command that cannot be parsed, which must not pass as a skip.
//
// Bare comma-separated test names, the other form the resolver accepts, become
// one anchored -run alternation over the whole module, so naming tests instead
// of a command is run rather than skipped.
func goTestArgs(evidence string) ([]string, error) {
	trimmed := strings.TrimSpace(evidence)
	if trimmed == "" {
		return nil, nil
	}
	if strings.HasPrefix(trimmed, "go test") {
		fields, ok := splitCommand(trimmed)
		if !ok {
			return nil, fmt.Errorf("unterminated quote; the command cannot be run as written")
		}
		if len(fields) < 3 {
			return nil, fmt.Errorf("names no package or test to run")
		}
		return fields[1:], nil
	}
	var names []string
	for _, name := range strings.Split(trimmed, ",") {
		name = strings.TrimSpace(name)
		if name == "" || !strings.HasPrefix(name, "Test") || strings.ContainsAny(name, " \t|/'\"") {
			return nil, nil // not bare test names; the resolver classifies it
		}
		names = append(names, "^"+name+"$")
	}
	if len(names) == 0 {
		return nil, nil
	}
	return []string{"test", "./...", "-run", strings.Join(names, "|")}, nil
}

// splitCommand splits a command line on whitespace, honoring single and double
// quotes so a -run regex containing '|' survives as one argument. It reports
// false on an unterminated quote rather than guessing where the argument ended.
func splitCommand(line string) ([]string, bool) {
	var fields []string
	var current strings.Builder
	var quote rune
	inField := false
	for _, r := range line {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
				continue
			}
			current.WriteRune(r)
		case r == '\'' || r == '"':
			quote = r
			inField = true
		case r == ' ' || r == '\t':
			if inField {
				fields = append(fields, current.String())
				current.Reset()
				inField = false
			}
		default:
			current.WriteRune(r)
			inField = true
		}
	}
	if quote != 0 {
		return nil, false
	}
	if inField {
		fields = append(fields, current.String())
	}
	return fields, true
}

func indentLines(text, prefix string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}
