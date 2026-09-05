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

func TestLoadDeclaredTools_ClosureAsAuthoredDedupedAndSorted(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	write := func(file, content string) string {
		t.Helper()
		path := filepath.Join(dir, file)
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
		return path
	}
	write("shared.yaml", `
tools:
  - name: read
    type: builtin
    description: Read a file.
  - name: zeta_tool
    type: builtin
    custom_note: authored field the runtime never interprets
`)
	write("request-tools.yaml", `
tools:
  - name: invoke_llm
    type: builtin
    provider: cohere
  - name: read
    type: builtin
    description: shadowed duplicate, first declaration wins
`)
	rootProfile := write("root-profile.yaml", `
name: root
machine: root.yaml
tools: [tools.yaml]
tool_declarations: [shared.yaml]
`)
	write("request-profile.yaml", `
name: request
machine: alpha.yaml
tools: [tools.yaml]
tool_declarations: [request-tools.yaml, shared.yaml]
`)

	defs := NewCollection()
	defs.Servers["requests"] = restdef.Server{Endpoints: map[string]restdef.Endpoint{
		"turn": {
			Binding: bindingMachineRequest,
			MachineRequest: restdef.MachineRequest{
				Profile: "request-profile.yaml",
				Machine: "alpha.yaml",
			},
		},
	}}

	declarations, err := LoadDeclaredTools(rootProfile, dir, defs)
	require.NoError(t, err)
	require.Len(t, declarations, 3)
	require.Equal(t, "invoke_llm", declarations[0]["name"])
	require.Equal(t, "cohere", declarations[0]["provider"])
	require.Equal(t, "read", declarations[1]["name"])
	require.Equal(t, "Read a file.", declarations[1]["description"])
	require.Equal(t, "zeta_tool", declarations[2]["name"])
	require.Equal(t, "authored field the runtime never interprets", declarations[2]["custom_note"])

	rootOnly, err := LoadDeclaredTools(rootProfile, dir, NewCollection())
	require.NoError(t, err)
	require.Len(t, rootOnly, 2)
	require.Equal(t, "read", rootOnly[0]["name"])
	require.Equal(t, "zeta_tool", rootOnly[1]["name"])
}
