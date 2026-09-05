// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package rest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	restdef "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/definition"
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

func TestSelectedDynamicDispatchVocabularyExcludesUnselectedDeclarations(t *testing.T) {
	defs := []catalog.ToolDef{
		{Name: "read", Visibility: "external"},
		{Name: "list_resource", Visibility: "external"},
		{Name: "invoke_llm", Visibility: "internal"},
	}
	got := selectedDynamicDispatchVocabulary(defs, []string{"read", "invoke_llm"})
	assert.Equal(t, []string{"read"}, got)
}

func TestLoadDeclaredMachines_RootFirstDeduplicatedAndSorted(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeMachine := func(name, file string) core.MachineSpec {
		t.Helper()
		path := filepath.Join(dir, file)
		data := []byte("name: " + name + `
initial_state: Idle
states: [Idle, Done]
terminal_states: [Done]
signals: [Seed]
transitions:
  - {state: Idle, signal: Seed, next: Done}
`)
		require.NoError(t, os.WriteFile(path, data, 0o600))
		machine, err := core.LoadMachineSpec(path)
		require.NoError(t, err)
		return machine
	}
	root := writeMachine("supervisor", "root.yaml")
	writeMachine("zeta", "zeta.yaml")
	writeMachine("alpha", "alpha.yaml")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "request-profile.yaml"), []byte(`
name: request
machine: alpha.yaml
tools: [tools.yaml]
`), 0o600))

	request := func(machine string) restdef.Endpoint {
		return restdef.Endpoint{
			Binding: bindingMachineRequest,
			MachineRequest: restdef.MachineRequest{
				Profile: "request-profile.yaml",
				Machine: machine,
			},
		}
	}
	defs := NewCollection()
	defs.Servers["requests"] = restdef.Server{Endpoints: map[string]restdef.Endpoint{
		"zeta":      request("zeta.yaml"),
		"alpha":     request("alpha.yaml"),
		"duplicate": request("zeta.yaml"),
		"root":      request("root.yaml"),
	}}

	machines, err := LoadDeclaredMachines(root, filepath.Join(dir, "root.yaml"), dir, defs)
	require.NoError(t, err)
	require.Equal(t, []string{"supervisor", "alpha", "zeta"}, []string{
		machines[0].Name, machines[1].Name, machines[2].Name,
	})

	rootOnly, err := LoadDeclaredMachines(root, filepath.Join(dir, "root.yaml"), dir, NewCollection())
	require.NoError(t, err)
	require.Len(t, rootOnly, 1)
	require.Equal(t, "supervisor", rootOnly[0].Name)
}
