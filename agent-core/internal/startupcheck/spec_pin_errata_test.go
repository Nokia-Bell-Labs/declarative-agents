// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package startupcheck

import (
	"testing"

	"github.com/stretchr/testify/require"

	internalload "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/load"
)

// TestSpecPinStartupRejectsAToolPhaseTheMachineLacks pins R-RUN-001's phase
// check: a selected tool whose declared phases name a state the loaded
// machine-profile does not declare refuses the start, while the same profile
// with a declared state passes.
func TestSpecPinStartupRejectsAToolPhaseTheMachineLacks(t *testing.T) {
	for _, tc := range []struct {
		name, phase, wantErr string
	}{
		{name: "declared state", phase: "Idle"},
		{name: "undeclared state", phase: "Nowhere", wantErr: `tool "wait" declares unknown phase "Nowhere"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			pinWrite(t, root, "machine.yaml", pinMachine("1m", "wait"))
			pinWrite(t, root, "tools.yaml", "tools: [wait]\n")
			pinWrite(t, root, "declarations.yaml", `unit: base
tools:
  - name: wait
    type: builtin
    init: custom_await
    category: boundary
    visibility: internal
    phases: [`+tc.phase+`]
    config: {timeout: 15s}
`)
			profile := pinWrite(t, root, "profile.yaml",
				"name: phased\nmachine: machine.yaml\ntools: [tools.yaml]\ntool_declarations: [declarations.yaml]\n")
			closure, err := internalload.LoadClosure(profile, internalload.Options{})
			require.NoError(t, err)

			_, err = ValidateClosure(closure)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}
