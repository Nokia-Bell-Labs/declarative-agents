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
		fmt.Printf("validated formal go_test evidence under %s\n", root)
		return nil
	}
	detail := strings.TrimSpace(string(out))
	if detail == "" {
		detail = err.Error()
	}
	return fmt.Errorf("formal go_test evidence validation failed:\n%s", detail)
}
