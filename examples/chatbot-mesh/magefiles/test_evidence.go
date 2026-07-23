// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"strings"
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

// runTestEvidence runs the tests this example's formal suites claim as evidence
// and fails the audit when one of them does not pass, by invoking the agent
// binary's --run-test-evidence preflight.
//
// Resolution and execution answer different questions. validateTestEvidence
// proves each named test exists, which catches a proof command pointing at
// another module or a renamed test. It runs nothing: `go test -list` compiles
// the test binaries and executes none of them. So a suite could record evidence
// for a test that fails and the audit stayed green -- which is what happened to
// the LLM preload gate case, whose evidence had never passed since the day it
// was written (GH-701, GH-713).
//
// The runner lives in agent-core (spec.RunGoTestEvidence) and is reached through
// the binary for the same reason the resolver is: this example does not import
// agent-core. It runs the module once and matches every claim against the
// results, so it also catches a claimed test that was skipped -- which a
// per-claim `go test -run` invocation cannot, since that exits zero whether the
// test passed or skipped (GH-717).
func runTestEvidence(run profileSmokeRunner, binary, root string) error {
	out, err := run(binary, "--run-test-evidence", "--directory", root)
	if err == nil {
		fmt.Printf("ran the go_test evidence this example claims under %s\n", root)
		return nil
	}
	detail := strings.TrimSpace(string(out))
	if detail == "" {
		detail = err.Error()
	}
	return fmt.Errorf("formal go_test evidence did not pass:\n%s", detail)
}
