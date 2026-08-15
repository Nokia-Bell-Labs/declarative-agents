// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"os"
	"regexp"
	"testing"
	"time"
)

// Evaluator-specific aggregate and nested-runner limits remain separate from
// the generic profile-closure inspector (GH-1669), as specified by GH-1668.
const (
	maxEvaluatorPointDeadline   = 15 * time.Minute
	criticPointProcessingMargin = time.Minute
	maxEvaluatorSessionDeadline = 24 * time.Hour
	benchSessionCleanupMargin   = time.Hour
)

func TestCatalogEvaluatorTimeoutEnvelopes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		machinePath    string
		action         string
		authority      time.Duration
		requiredMargin time.Duration
	}{
		{
			name: "critic point child runtime", machinePath: "../agents/critic/point.yaml",
			action: "run_agent", authority: maxEvaluatorPointDeadline, requiredMargin: criticPointProcessingMargin,
		},
		{
			name: "critic session run_point", machinePath: "../agents/critic/machine.yaml",
			action: "run_point", authority: maxEvaluatorPointDeadline, requiredMargin: criticPointProcessingMargin,
		},
		{
			name: "bench evaluator child session", machinePath: "../agents/bench/machine.yaml",
			action: "launch_evaluator", authority: maxEvaluatorSessionDeadline, requiredMargin: benchSessionCleanupMargin,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got := readEvaluatorMachineCommandTimeout(t, test.machinePath)
			if minimum := test.authority + test.requiredMargin; got < minimum {
				t.Errorf("%s command_timeout = %s, must be at least %s (%s authority + %s margin)",
					test.machinePath, got, minimum, test.authority, test.requiredMargin)
			}
			requireEvaluatorMachineAction(t, test.machinePath, test.action)
		})
	}
}

func readEvaluatorMachineCommandTimeout(t *testing.T, path string) time.Duration {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`command_timeout:\s*([0-9]+(?:ns|us|µs|ms|s|m|h))`).FindSubmatch(data)
	if len(match) != 2 {
		t.Fatalf("%s has no command_timeout", path)
	}
	timeout, err := time.ParseDuration(string(match[1]))
	if err != nil {
		t.Fatalf("%s command_timeout: %v", path, err)
	}
	return timeout
}

func requireEvaluatorMachineAction(t *testing.T, path, action string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	pattern := regexp.MustCompile(`(?m)^\s*action:\s*` + regexp.QuoteMeta(action) + `\s*$`)
	if !pattern.Match(data) {
		t.Errorf("%s does not dispatch action %q", path, action)
	}
}
