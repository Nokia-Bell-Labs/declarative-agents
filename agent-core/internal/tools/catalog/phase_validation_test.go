// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package catalog

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func TestValidateToolPhasesRejectsUnknownState(t *testing.T) {
	t.Parallel()
	machine := core.MachineSpec{
		States: core.StateSpecsFromNames("Composing", "Done"),
	}
	err := ValidateToolPhases(machine, []ToolDef{{
		Name: "write", Phases: []string{"Compsing"},
	}})
	require.ErrorContains(t, err, `tool "write" declares unknown phase "Compsing"`)
}

func TestValidateToolPhasesAcceptsDeclaredAndUnscopedWords(t *testing.T) {
	t.Parallel()
	machine := core.MachineSpec{
		States: core.StateSpecsFromNames("Composing", "Done"),
	}
	require.NoError(t, ValidateToolPhases(machine, []ToolDef{
		{Name: "write", Phases: []string{"Composing"}},
		{Name: "done"},
	}))
}
