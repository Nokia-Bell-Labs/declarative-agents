// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

func TestMachineHasDynamicDispatch(t *testing.T) {
	with := core.MachineSpec{Transitions: []core.TransitionSpec{
		{State: "Parsing", Signal: "ToolDone", Next: "Answering", Action: "$tool"},
	}}
	without := core.MachineSpec{Transitions: []core.TransitionSpec{
		{State: "A", Signal: "S", Next: "B", Action: "invoke_llm"},
	}}
	assert.True(t, machineHasDynamicDispatch(with))
	assert.False(t, machineHasDynamicDispatch(without))
}

func TestDynamicDispatchVocabulary(t *testing.T) {
	defs := []catalog.ToolDef{
		{Name: "embed_query", Visibility: "internal"},
		{Name: "invoke_llm_fast", Visibility: "external"},
		{Name: "invoke_llm_deep"}, // empty visibility defaults to external
	}
	got := dynamicDispatchVocabulary(defs)
	assert.ElementsMatch(t, []string{"invoke_llm_fast", "invoke_llm_deep"}, got)
}
