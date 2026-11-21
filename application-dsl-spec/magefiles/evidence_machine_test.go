// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"path/filepath"
	"testing"
)

// machineFixtureChecks pairs each machine-format and fragment check with the
// fixture directory its statement owns; every directory holds one valid and
// one invalid fixture.
var machineFixtureChecks = []struct {
	dir   string
	check func(string) error
}{
	{"machine-top-keynames", checkMachineTopKeynames},
	{"machine-transition-keynames", checkMachineTransitionKeynames},
	{"machine-named-specs", checkMachineNamedSpecs},
	{"machine-graph", checkMachineGraphReferences},
	{"machine-run-status", checkMachineRunStatus},
	{"machine-budget", checkMachineBudget},
	{"machine-policy-signals", checkMachinePolicySignals},
	{"machine-action-forms", checkMachineActionForms},
	{"machine-step-labels", checkMachineStepLabels},
	{"machine-for-each-shape", checkMachineForEachShape},
	{"machine-for-each-policy", checkMachineForEachPolicy},
	{"model-stage-fragment", checkModelStageFragment},
	{"model-machine-template", checkModelMachineTemplate},
	{"model-fragment-params", checkModelFragmentParams},
}

func TestMachineFixtureChecksHonorDeclaredVerdicts(t *testing.T) {
	t.Parallel()
	for _, tc := range machineFixtureChecks {
		dir := filepath.Join("..", "1.0", "fixtures", "document", tc.dir)
		if err := tc.check(filepath.Join(dir, "valid.yaml")); err != nil {
			t.Errorf("%s/valid.yaml failed its check: %v", tc.dir, err)
		}
		if err := tc.check(filepath.Join(dir, "invalid.yaml")); err == nil {
			t.Errorf("%s/invalid.yaml passed its check", tc.dir)
		}
	}
}

// TestMachineChecksRejectEachViolation exercises the branches the invalid
// fixtures do not reach, so a check cannot pass a rule it claims to enforce.
func TestMachineChecksRejectEachViolation(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		check func(string) error
		body  string
	}{
		"top-level unknown": {checkMachineTopKeynames, "name: m\ninitial: Idle\n"},
		"transition no state": {checkMachineTransitionKeynames,
			"transitions:\n  - {signal: Seed, next: Done}\n"},
		"signal long-form unknown": {checkMachineNamedSpecs,
			"states: [Idle]\nsignals:\n  - {name: Seed, triggr: typo}\n"},
		"duplicate state": {checkMachineNamedSpecs, "states: [Idle, Idle]\nsignals: [Seed]\n"},
		"initial undeclared": {checkMachineGraphReferences,
			"initial_state: Nowhere\nstates: [Idle, Done]\nterminal_states: [Done]\nsignals: [Seed]\n" +
				"transitions:\n  - {state: Idle, signal: Seed, next: Done}\n"},
		"terminal undeclared": {checkMachineGraphReferences,
			"initial_state: Idle\nstates: [Idle, Done]\nterminal_states: [Gone]\nsignals: [Seed]\n" +
				"transitions:\n  - {state: Idle, signal: Seed, next: Done}\n"},
		"no transitions": {checkMachineGraphReferences,
			"initial_state: Idle\nstates: [Idle, Done]\nterminal_states: [Done]\nsignals: [Seed]\ntransitions: []\n"},
		"signal undeclared": {checkMachineGraphReferences,
			"initial_state: Idle\nstates: [Idle, Done]\nterminal_states: [Done]\nsignals: [Seed]\n" +
				"transitions:\n  - {state: Idle, signal: Go, next: Done}\n"},
		"duplicate pair": {checkMachineGraphReferences,
			"initial_state: Idle\nstates: [Idle, Done]\nterminal_states: [Done]\nsignals: [Seed]\n" +
				"transitions:\n  - {state: Idle, signal: Seed, next: Done}\n  - {state: Idle, signal: Seed, next: Idle}\n"},
		"unknown run status": {checkMachineRunStatus,
			"states: [{name: Done, run_status: finished}]\nterminal_states: [Done]\n"},
		"terminal without status": {checkMachineRunStatus, "states: [Idle, Done]\nterminal_states: [Done]\n"},
		"budget unknown keyname": {checkMachineBudget,
			"budget: {max_iterations: 3, command_timeout: 1m, max_retries: 2}\n"},
		"budget zero iterations": {checkMachineBudget, "budget: {max_iterations: 0, command_timeout: 1m}\n"},
		"budget bad timeout":     {checkMachineBudget, "budget: {max_iterations: 3, command_timeout: never}\n"},
		"summary undeclared":     {checkMachinePolicySignals, "summary_signal: Missing\nsignals: [Seed]\n"},
		"summary without action": {checkMachineActionForms,
			"transitions:\n  - {state: Idle, signal: Seed, next: Done, summary: true}\n"},
		"report_output selector": {checkMachineActionForms,
			"transitions:\n  - {state: Idle, signal: Seed, next: Done, action: a, report_output: $from(x).y}\n"},
		"empty label": {checkMachineStepLabels,
			"transitions:\n  - {state: Idle, signal: Seed, next: Done, action: a, label: \"\"}\n"},
		"duplicate external label": {checkMachineStepLabels, "external_labels: [seed, seed]\ntransitions: []\n"},
		"for_each dynamic action": {checkMachineForEachShape,
			"states: [A, Done]\nterminal_states: [Done]\nsignals: [Go, Ok, All, Fail, None]\ntransitions:\n" +
				"  - {state: A, signal: Go, next: A, action: $tool, for_each: {items: $from(x).y, as: i, continue_on: [Ok]," +
				" join: {next: Done, signals: {all_success: All, failed: Fail, empty: None}}}}\n"},
		"for_each missing empty": {checkMachineForEachShape,
			"states: [A, Done]\nterminal_states: [Done]\nsignals: [Go, Ok, All, Fail]\ntransitions:\n" +
				"  - {state: A, signal: Go, next: A, action: a, for_each: {items: $from(x).y, as: i, continue_on: [Ok]," +
				" join: {next: Done, signals: {all_success: All, failed: Fail}}}}\n"},
		"for_each no continue_on": {checkMachineForEachShape,
			"states: [A, Done]\nterminal_states: [Done]\nsignals: [Go, All, Fail, None]\ntransitions:\n" +
				"  - {state: A, signal: Go, next: A, action: a, for_each: {items: $from(x).y, as: i," +
				" join: {next: Done, signals: {all_success: All, failed: Fail, empty: None}}}}\n"},
		"sequential bound": {checkMachineForEachPolicy,
			"transitions:\n  - {state: A, signal: Go, next: A, action: a, for_each: {mode: sequential, max_concurrency: 2}}\n"},
		"unknown failure": {checkMachineForEachPolicy,
			"transitions:\n  - {state: A, signal: Go, next: A, action: a, for_each: {failure: best_effort}}\n"},
		"collect_all no partial": {checkMachineForEachPolicy,
			"transitions:\n  - {state: A, signal: Go, next: A, action: a, for_each: {failure: collect_all," +
				" join: {signals: {all_success: All}}}}\n"},
		"stage without params": {checkModelStageFragment, "unit: s\nstage:\n  transitions: []\n"},
		"template body imports": {checkModelMachineTemplate,
			"unit: t\nparams: []\nmachine:\n  name: m\n  imports: [x.yaml]\n"},
		"param unknown type":   {checkModelFragmentParams, "params:\n  - {name: n, type: list}\n"},
		"param duplicate name": {checkModelFragmentParams, "params:\n  - {name: n, type: string}\n  - {name: n, type: string}\n"},
		"param default enum": {checkModelFragmentParams,
			"params:\n  - {name: n, type: string, enum: [a, b], default: c}\n"},
	} {
		path := writeEvidenceFile(t, t.TempDir(), "doc.yaml", tc.body)
		if err := tc.check(path); err == nil {
			t.Errorf("%s: check passed a violating document", name)
		}
	}
}
