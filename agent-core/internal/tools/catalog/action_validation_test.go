// Copyright (c) 2026 Nokia. All rights reserved.

package catalog

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func TestValidateMachineActionsAcceptsSelectedAndSpecialActions(t *testing.T) {
	t.Parallel()

	spec := core.MachineSpec{
		Name: "valid-actions",
		Transitions: []core.TransitionSpec{
			{State: "Idle", Signal: "Seed", Action: "selected"},
			{State: "Working", Signal: "ToolDone", Action: "$tool"},
			{State: "Finishing", Signal: "Done"},
		},
	}

	require.NoError(t, ValidateMachineActions(spec, []ToolDef{{Name: "selected"}}))
}

func TestValidateMachineActionsRejectsUnselectedLiteralActions(t *testing.T) {
	t.Parallel()

	spec := core.MachineSpec{
		Name: "inactive-actions",
		Transitions: []core.TransitionSpec{
			{State: "Idle", Signal: "Seed", Action: "active"},
			{State: "Working", Signal: "Ready", Action: "declared_but_inactive"},
			{State: "Checking", Signal: "Ready", Action: "missing"},
		},
	}

	err := ValidateMachineActions(spec, []ToolDef{{Name: "active"}})

	require.ErrorContains(t, err, "machine action validation")
	require.ErrorContains(t, err, `transition[1] Working/Ready action "declared_but_inactive" is not selected`)
	require.ErrorContains(t, err, `transition[2] Checking/Ready action "missing" is not selected`)
}
