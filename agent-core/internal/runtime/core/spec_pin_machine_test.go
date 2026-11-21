// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package core

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// These tests pin machine-format load rules that the application-dsl-spec 1.0
// statements record (R-MACH family) where no other test asserts them.

// TestSpecPinMachineKeynamesAreAClosedSet pins R-MACH-003 and R-MACH-008: a
// machine document and its budget accept only the keynames the format names.
func TestSpecPinMachineKeynamesAreAClosedSet(t *testing.T) {
	t.Parallel()
	for name, input := range map[string]string{
		"top-level": strings.Replace(validYAML, "name: test-machine", "name: test-machine\ninitial: Idle", 1),
		"budget": strings.Replace(validYAML, "budget: {max_iterations: 100, command_timeout: 1m}",
			"budget: {max_iterations: 100, command_timeout: 1m, max_retries: 3}", 1),
	} {
		_, err := ParseMachineSpec([]byte(input))
		require.ErrorContains(t, err, "field", name)
	}
	_, err := ParseMachineSpec([]byte(validYAML))
	require.NoError(t, err)
}

// TestSpecPinMachineRequiresAWellFormedGraph pins R-MACH-006:
// the grammar is non-empty, the initial state is declared, and a transition's
// next state is declared.
func TestSpecPinMachineRequiresAWellFormedGraph(t *testing.T) {
	t.Parallel()
	_, err := ParseMachineSpec([]byte("name: empty\ninitial_state: Idle\nstates: []\nterminal_states: []\nsignals: []\ntransitions: []\n"))
	require.ErrorContains(t, err, "at least one state is required")
	require.ErrorContains(t, err, "at least one terminal_state is required")
	require.ErrorContains(t, err, "at least one transition is required")

	_, err = ParseMachineSpec([]byte(strings.Replace(validYAML, "initial_state: Idle", "initial_state: Nowhere", 1)))
	require.ErrorContains(t, err, `initial_state "Nowhere" not in states list`)

	_, err = ParseMachineSpec([]byte(strings.Replace(validYAML, "next: Running", "next: Nowhere", 1)))
	require.ErrorContains(t, err, `next "Nowhere" not in states list`)
}

// TestSpecPinMachineStepLabelGrammar pins R-MACH-013: a present transition
// label or runtime-seeded label is a non-empty name free of dots,
// parentheses, whitespace, and control characters; transition labels may
// repeat, runtime-seeded labels may not.
func TestSpecPinMachineStepLabelGrammar(t *testing.T) {
	t.Parallel()
	for _, label := range []string{`""`, "a.b", "a(b)", `"a b"`} {
		input := strings.Replace(validYAML, "    action: do_work", "    action: do_work\n    label: "+label, 1)
		_, err := ParseMachineSpec([]byte(input))
		require.ErrorContains(t, err, "is not a valid command-state label", label)
	}

	repeated := strings.Replace(validYAML, "    action: do_work", "    action: do_work\n    label: step", 1)
	repeated = strings.Replace(repeated, "    next: Done", "    next: Done\n    action: finish\n    label: step", 1)
	_, err := ParseMachineSpec([]byte(repeated))
	require.NoError(t, err, "duplicate transition labels are valid")

	_, err = ParseMachineSpec([]byte(strings.Replace(validYAML, "name: test-machine",
		"name: test-machine\nexternal_labels: [seed, seed]", 1)))
	require.ErrorContains(t, err, `duplicate label "seed"`)
	_, err = ParseMachineSpec([]byte(strings.Replace(validYAML, "name: test-machine",
		"name: test-machine\nexternal_labels: [bad.label]", 1)))
	require.ErrorContains(t, err, "is not a valid command-state label")
}

// TestSpecPinForEachJoinAndFailurePolicy pins R-MACH-016 and R-MACH-017: the
// rules of for_each that TestMachineSpecForEachValidation leaves unasserted.
func TestSpecPinForEachJoinAndFailurePolicy(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct{ old, repl, want string }{
		"failure enum": {
			"failure: fail_fast", "failure: best_effort",
			`failure: "best_effort" must be fail_fast or collect_all`,
		},
		"collect_all partial": {
			"failure: fail_fast", "failure: collect_all",
			"join.signals.partial: required for collect_all",
		},
		"continue_on": {
			"continue_on: [ItemDone]", "continue_on: []",
			"continue_on: at least one signal is required",
		},
		"join signal required": {
			"          empty: ItemsEmpty\n", "",
			"join.signals.empty: signal is required",
		},
		"join signal declared": {
			"empty: ItemsEmpty", "empty: NeverDeclared",
			`join.signals.empty: signal "NeverDeclared" not in signals list`,
		},
		"abort_on declared": {
			"abort_on: [CommandError]", "abort_on: [NeverDeclared]",
			`signal "NeverDeclared" not in signals list`,
		},
		"join routing": {
			"  - state: Joined\n    signal: ItemsEmpty\n    next: Done\n", "",
			`for_each.join: no transition for state "Joined" and signal "ItemsEmpty"`,
		},
	} {
		input := strings.Replace(iteratorMachineYAML, tc.old, tc.repl, 1)
		require.NotEqual(t, iteratorMachineYAML, input, name)
		_, err := ParseMachineSpec([]byte(input))
		require.ErrorContains(t, err, tc.want, name)
	}

	terminalJoin := strings.Replace(iteratorMachineYAML, "        next: Joined\n", "        next: Done\n", 1)
	_, err := ParseMachineSpec([]byte(terminalJoin))
	require.NoError(t, err, "a join into a terminal state needs no routing")
}
