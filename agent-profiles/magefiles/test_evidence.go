// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"strings"
)

// validateTestEvidence resolves every formal test suite's go_test evidence in
// this module against its real Go tests, by invoking the agent binary's
// --validate-test-evidence preflight.
//
// The resolver lives in agent-core (spec.AuditGoTestEvidence) and this module
// deliberately does not import agent-core, so the check is reached through the
// binary the audit already builds — the same way the boot smoke reuses
// --validate-config. Without it, a suite command such as
// `go test ./magefiles -run TestThatMovedAway` exits green while running no
// test, and the documented proof is worthless (GH-592, GH-652).
func validateTestEvidence(run profileSmokeRunner, binary, root string) error {
	out, err := run(binary, "--validate-test-evidence", "--directory", root)
	if err == nil {
		fmt.Printf("validated formal go_test evidence under %s\n", root)
		return nil
	}
	detail := strings.TrimSpace(string(out))
	if detail == "" {
		detail = err.Error()
	}
	return fmt.Errorf("formal go_test evidence validation failed:\n%s", detail)
}
