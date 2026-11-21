// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package startupcheck

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	internalload "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/load"
)

// TestStartupBoundaryRejectsAFaultInAReachedChildMachine pins the 1.0
// specification's R-RUN statement on reached machines: the startup boundary
// applies to every machine the profile closure reaches, so a wiring fault in a
// child agent profile's machine refuses the parent's start, though the
// parent's own machine is sound.
func TestStartupBoundaryRejectsAFaultInAReachedChildMachine(t *testing.T) {
	for _, tc := range []struct {
		name        string
		childAction string
		wantErr     string
	}{
		{name: "sound child", childAction: "child_wait"},
		{name: "child names an unselected action", childAction: "missing_word", wantErr: `action "missing_word" is not selected`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			profile := childWrapperProfile(t, tc.childAction)
			closure, err := internalload.LoadClosure(profile, internalload.Options{})
			require.NoError(t, err, "the parent closure itself loads")

			_, err = ValidateClosure(closure)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.wantErr)
			require.ErrorContains(t, err, filepath.Join("child", "machine.yaml"))
		})
	}
}

func childWrapperProfile(t *testing.T, childAction string) string {
	t.Helper()
	root := t.TempDir()
	child := filepath.Join(root, "child")
	require.NoError(t, os.Mkdir(child, 0o755))
	pinWrite(t, child, "machine.yaml", pinMachine("1m", childAction))
	pinWrite(t, child, "tools.yaml", "tools: [child_wait]\n")
	pinWrite(t, child, "declarations.yaml", `unit: base
tools:
  - name: child_wait
    type: builtin
    init: custom_await
    category: boundary
    visibility: internal
    config: {timeout: 15s}
`)
	pinWrite(t, child, "profile.yaml",
		"name: child\nmachine: machine.yaml\ntools: [tools.yaml]\ntool_declarations: [declarations.yaml]\n")

	pinWrite(t, root, "machine.yaml", pinMachine("20m", "invoke_executor"))
	pinWrite(t, root, "tools.yaml", "tools: [invoke_executor]\n")
	pinWrite(t, root, "declarations.yaml", `unit: base
tools:
  - name: invoke_executor
    type: builtin
    init: self_invoke
    category: boundary
    visibility: internal
    config: {profile: child/profile.yaml}
`)
	return pinWrite(t, root, "profile.yaml",
		"name: parent\nmachine: machine.yaml\ntools: [tools.yaml]\ntool_declarations: [declarations.yaml]\n")
}

func pinMachine(commandTimeout, action string) string {
	return `
name: fixture
initial_state: Idle
budget: {max_iterations: 2, command_timeout: ` + commandTimeout + `}
states: [Idle, {name: Done, run_status: succeeded}]
terminal_states: [Done]
signals: [Seed]
transitions:
  - {state: Idle, signal: Seed, next: Done, action: ` + action + `}
`
}

func pinWrite(t *testing.T, root, name, content string) string {
	t.Helper()
	path := filepath.Join(root, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}
