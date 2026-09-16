// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package profileaudit

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

// twoRouteRequestProfile dispatches one request machine from two endpoints,
// each seeding it with its own signal. The walk visits that machine once, so
// the second endpoint's signal is only observable if it is recorded before the
// visit check (GH-2058).
func twoRouteRequestProfile(t *testing.T, root string) string {
	t.Helper()
	write(t, root, "machine.yaml", oneActionMachine("1m", "launch"))
	write(t, root, "request-machine.yaml", oneActionMachine("10s", "request_wait"))
	write(t, root, "tools.yaml", "tools: [launch]\n")
	write(t, root, "declarations.yaml", declarations(`
  - name: launch
    type: builtin
    init: rest_server_launch
    category: boundary
    visibility: internal
    config: {rest_ref: api}
`+tool("request_wait", "custom_await", "10s", "internal")))
	write(t, root, "rest.yaml", `
rest:
  version: v1
  limits:
    local: {timeout: 30s}
  servers:
    api:
      address: 127.0.0.1:19000
      limits_ref: local
      endpoints:
        start:
          method: POST
          path: /start
          binding: machine_request
          machine_request:
            profile: profile.yaml
            machine: request-machine.yaml
            initial_signal: StartRequested
            timeout: 1m
            response:
              terminal_states:
                Done: {status: 200}
        resume:
          method: POST
          path: /resume
          binding: machine_request
          machine_request:
            profile: profile.yaml
            machine: request-machine.yaml
            initial_signal: ResumeRequested
            timeout: 1m
            response:
              terminal_states:
                Done: {status: 200}
`)
	return writeProfile(t, root, "profile.yaml", "machine.yaml", "tools.yaml",
		"declarations.yaml", "rest_definitions: [rest.yaml]\n")
}

func TestReachedMachinesUniteTheSignalsEveryRouteInjects(t *testing.T) {
	root := t.TempDir()
	profile := twoRouteRequestProfile(t, root)

	var reached []ReachedMachine
	_, err := InspectWithOptions(profile, Options{
		OnMachine: func(machine ReachedMachine) error {
			reached = append(reached, machine)
			return nil
		},
	})

	require.NoError(t, err)
	require.Len(t, reached, 2, "the profile's own machine and the request machine it dispatches")
	request := reached[1]
	require.Equal(t, canonical(filepath.Join(root, "request-machine.yaml")), request.MachinePath)
	require.True(t, request.RequestScoped)
	require.Equal(t, []string{"ResumeRequested", "StartRequested"}, request.InitialSignals,
		"both routes seed the one machine the walk visits once")
}

func TestReachedProfileMachineIsNotRequestScoped(t *testing.T) {
	root := t.TempDir()
	profile := twoRouteRequestProfile(t, root)

	var reached []ReachedMachine
	_, err := InspectWithOptions(profile, Options{
		OnMachine: func(machine ReachedMachine) error {
			reached = append(reached, machine)
			return nil
		},
	})

	require.NoError(t, err)
	own := reached[0]
	require.Equal(t, canonical(filepath.Join(root, "machine.yaml")), own.MachinePath)
	require.False(t, own.RequestScoped)
	require.Empty(t, own.InitialSignals, "nothing injects a signal into the machine a profile runs")
}

// TestReachedMachineCarriesWhatThatMachineSelects covers the reason the walk is
// reused rather than rebuilt: a request machine selects the actions its own
// transitions name, so its tools are not the profile's.
func TestReachedMachineCarriesWhatThatMachineSelects(t *testing.T) {
	root := t.TempDir()
	profile := twoRouteRequestProfile(t, root)

	var reached []ReachedMachine
	_, err := InspectWithOptions(profile, Options{
		OnMachine: func(machine ReachedMachine) error {
			reached = append(reached, machine)
			return nil
		},
	})

	require.NoError(t, err)
	require.Equal(t, []string{"launch"}, toolNames(reached[0].Selected))
	require.Equal(t, []string{"request_wait"}, toolNames(reached[1].Selected))
}

func toolNames(defs []catalog.ToolDef) []string {
	names := make([]string, 0, len(defs))
	for _, def := range defs {
		names = append(names, def.Name)
	}
	return names
}

// wrapperProfile declares a profile that reaches a child agent profile through
// self_invoke and an evaluator point machine through run_point.
func wrapperProfile(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	child := filepath.Join(root, "child")
	require.NoError(t, os.Mkdir(child, 0o755))
	write(t, child, "machine.yaml", oneActionMachine("15s", "child_wait"))
	write(t, child, "tools.yaml", "tools: [child_wait]\n")
	write(t, child, "declarations.yaml", declarations(tool("child_wait", "custom_await", "15s", "internal")))
	writeProfile(t, child, "profile.yaml", "machine.yaml", "tools.yaml", "declarations.yaml", "")

	write(t, root, "machine.yaml", `
name: wrappers
initial_state: S0
budget: {max_iterations: 5, command_timeout: 10m}
states: [S0, S1, {name: Done, run_status: succeeded}]
terminal_states: [Done]
signals: [Seed, ChildDone, PointDone]
transitions:
  - {state: S0, signal: Seed, next: S1, action: invoke_executor}
  - {state: S1, signal: ChildDone, next: Done, action: evaluate_point}
`)
	write(t, root, "point.yaml", oneActionMachine("20s", "point_wait"))
	write(t, root, "point-tools.yaml", "tools: [point_wait]\n")
	write(t, root, "point-declarations.yaml", declarations(tool("point_wait", "custom_await", "20s", "internal")))
	write(t, root, "tools.yaml", "tools: [invoke_executor, evaluate_point]\n")
	write(t, root, "declarations.yaml", declarations(`
  - name: invoke_executor
    type: builtin
    init: self_invoke
    category: boundary
    visibility: internal
    config: {profile: child/profile.yaml}
  - name: evaluate_point
    type: builtin
    init: run_point
    category: boundary
    visibility: internal
    config:
      point_machine: point.yaml
      point_tools: point-tools.yaml
      point_tool_declarations: [point-declarations.yaml]
      agent_name: point
      max_iterations: 5
      success_state: Done
`))
	return root, writeProfile(
		t, root, "profile.yaml", "machine.yaml", "tools.yaml", "declarations.yaml", "")
}

// TestReachedMachinesIncludeTheChildAndThePointMachine covers GH-2060. A child
// profile runs the boundary in its own process; a point machine runs in the
// evaluator's through core.Loop and nothing applies the boundary to it, so
// both have to reach a caller that checks wiring.
func TestReachedMachinesIncludeTheChildAndThePointMachine(t *testing.T) {
	root, profile := wrapperProfile(t)

	var reached []ReachedMachine
	_, err := InspectWithOptions(profile, Options{
		OnMachine: func(machine ReachedMachine) error {
			reached = append(reached, machine)
			return nil
		},
	})

	require.NoError(t, err)
	paths := make([]string, 0, len(reached))
	for _, machine := range reached {
		paths = append(paths, machine.MachinePath)
		require.False(t, machine.RequestScoped,
			"nothing here is dispatched by an endpoint, so nothing takes the request seed")
	}
	require.ElementsMatch(t, []string{
		canonical(filepath.Join(root, "machine.yaml")),
		canonical(filepath.Join(root, "child", "machine.yaml")),
		canonical(filepath.Join(root, "point.yaml")),
	}, paths)
}

// TestReachedPointMachineCarriesItsOwnTools keeps the point machine's
// selection distinct: it selects what its own point_tools names, not the
// profile's.
func TestReachedPointMachineCarriesItsOwnTools(t *testing.T) {
	root, profile := wrapperProfile(t)

	var point ReachedMachine
	_, err := InspectWithOptions(profile, Options{
		OnMachine: func(machine ReachedMachine) error {
			if machine.MachinePath == canonical(filepath.Join(root, "point.yaml")) {
				point = machine
			}
			return nil
		},
	})

	require.NoError(t, err)
	require.Equal(t, []string{"point_wait"}, toolNames(point.Selected))
}
