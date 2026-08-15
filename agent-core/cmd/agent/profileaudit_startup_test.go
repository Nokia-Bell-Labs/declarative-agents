// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStartupEnforcesProfileTimeoutClosureForRunAndValidation(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "profile.yaml"), `
name: timeout-fixture
machine: machine.yaml
tools: [tools.yaml]
tool_declarations: [declarations.yaml]
`)
	writeTestFile(t, filepath.Join(root, "machine.yaml"), `
name: timeout-fixture
initial_state: Idle
budget: {max_iterations: 2, command_timeout: 30s}
states: [Idle, {name: Done, run_status: succeeded}]
terminal_states: [Done]
signals: [Seed]
transitions:
  - {state: Idle, signal: Seed, next: Done, action: wait}
`)
	writeTestFile(t, filepath.Join(root, "tools.yaml"), "tools: [wait]\n")
	writeTestFile(t, filepath.Join(root, "declarations.yaml"), `
tools:
  - name: wait
    type: builtin
    init: custom_await
    category: boundary
    visibility: internal
    config: {timeout: 30s}
`)

	snapshot := snapshotAgentFlags()
	t.Cleanup(func() { restoreAgentFlags(snapshot) })
	for _, validateOnly := range []bool{false, true} {
		clearAgentFlags()
		flagProfile = filepath.Join(root, "profile.yaml")
		flagValidateConfig = validateOnly
		err := run(rootCmd, nil)
		require.ErrorContains(t, err, "inspect profile timeout closure")
		require.ErrorContains(t, err, "operation duration must be strictly below command_timeout")
	}
}
