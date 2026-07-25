// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"strings"
)

// validateTestEvidence resolves formal evidence claims without executing live
// model stages. Planned cases intentionally carry no go_test claim.
func validateTestEvidence(binary, root string) error {
	if err := runAgentPreflight(binary, "--validate-test-evidence", "--directory", root); err != nil {
		return fmt.Errorf("formal go_test evidence validation failed: %w", err)
	}
	return nil
}

// runTestEvidence executes only implemented Go-test claims. Ollama-gated Mage
// targets are not represented as passed evidence merely because they skipped.
func runTestEvidence(binary, root string) error {
	if err := runAgentPreflight(binary, "--run-test-evidence", "--directory", root); err != nil {
		if strings.Contains(err.Error(), "no runnable go_test evidence found") {
			fmt.Println("SKIP formal test-evidence execution: no runnable go_test claims; live target outcomes are not converted into passed evidence when model prerequisites skip")
			return nil
		}
		return fmt.Errorf("formal go_test evidence did not pass: %w", err)
	}
	return nil
}
