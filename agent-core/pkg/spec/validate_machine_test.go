// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import (
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

func TestValidate_MachineNodesInGraph(t *testing.T) {
	g, c := loadTestGraphAndCorpus(t)

	require.Contains(t, c.Machines, "test-agent", "test fixture should have a test-agent machine")

	machineNodes := g.NodesByKind(KindMachine)
	require.NotEmpty(t, machineNodes, "graph should contain machine nodes")

	var found bool
	for _, n := range machineNodes {
		if n.Machine == "test-agent" {
			found = true
			break
		}
	}
	assert.True(t, found, "graph should contain test-agent machine node")

	stateNodes := g.NodesByKind(KindMachineState)
	assert.GreaterOrEqual(t, len(stateNodes), 4, "test-agent has 4 states")

	signalNodes := g.NodesByKind(KindMachineSignal)
	assert.GreaterOrEqual(t, len(signalNodes), 3, "test-agent has 3 signals")

	transitionNodes := g.NodesByKind(KindMachineTransition)
	assert.GreaterOrEqual(t, len(transitionNodes), 3, "test-agent has 3 transitions")
}

func TestValidate_MachineContainsEdges(t *testing.T) {
	g, _ := loadTestGraphAndCorpus(t)

	outgoing := g.OutgoingByRel("machine:test-agent", RelContains)
	assert.GreaterOrEqual(t, len(outgoing), 10, "machine should contain states + signals + transitions")
}

func TestValidate_MachineActionResolution(t *testing.T) {
	corpus := &Corpus{
		Machines: map[string]core.MachineSpec{
			"good": {
				Name:         "good",
				InitialState: "Idle",
				States:       core.StateSpecs{{Name: "Idle"}, {Name: "Done"}},
				Signals:      core.SignalSpecs{{Name: "Seed"}},
				Transitions: []core.TransitionSpec{
					{State: "Idle", Signal: "Seed", Next: "Done", Action: "work"},
				},
				TerminalStates: []string{"Done"},
			},
			"bad": {
				Name:         "bad",
				InitialState: "Idle",
				States:       core.StateSpecs{{Name: "Idle"}, {Name: "Done"}},
				Signals:      core.SignalSpecs{{Name: "Seed"}},
				Transitions: []core.TransitionSpec{
					{State: "Idle", Signal: "Seed", Next: "Done", Action: "missing_tool"},
				},
				TerminalStates: []string{"Done"},
			},
		},
		ToolSelections: map[string][]string{
			"good": {"work"},
			"bad":  {"other_tool"},
		},
		MachineOrder: []string{"bad", "good"},
	}

	findings := checkMachineActionResolution(corpus)

	require.Len(t, findings, 1, "only 'bad' should have an unresolved action")
	for _, f := range findings {
		assert.Equal(t, "error", f.Level)
	}
	assert.Contains(t, findings[0].Message, "missing_tool")
}

func TestValidate_MachineMetricLabels(t *testing.T) {
	corpus := &Corpus{
		Machines: map[string]core.MachineSpec{
			"bad-machine": {
				Name:         "bad-machine",
				MetricLabels: core.MetricLabels{"phase": "request_id"},
			},
		},
	}

	findings := checkMachineMetricLabels(corpus)

	require.Len(t, findings, 1)
	assert.Equal(t, "machine-metric-label-invalid", findings[0].Check)
	assert.Contains(t, findings[0].Message, "request_id")
}

func TestValidate_MachineActionResolutionIgnoresDynamicTool(t *testing.T) {
	corpus := &Corpus{
		Machines: map[string]core.MachineSpec{
			"agent": {
				Name:         "agent",
				InitialState: "Idle",
				States:       core.StateSpecs{{Name: "Idle"}, {Name: "Dispatching"}},
				Signals:      core.SignalSpecs{{Name: "Seed"}, {Name: "ToolDone"}},
				Transitions: []core.TransitionSpec{
					{State: "Idle", Signal: "Seed", Next: "Dispatching", Action: "$tool"},
					{State: "Dispatching", Signal: "ToolDone", Next: "Dispatching"},
				},
				TerminalStates: []string{"Dispatching"},
			},
		},
		ToolSelections: map[string][]string{
			"agent": {"read"},
		},
		MachineOrder: []string{"agent"},
	}

	findings := checkMachineActionResolution(corpus)
	assert.Empty(t, findings, "$tool is dynamic dispatch and should not require a tool named $tool")
}

func TestValidate_MachineSignalCoverage(t *testing.T) {
	corpus := &Corpus{
		Machines: map[string]core.MachineSpec{
			"sig-test": {
				Name:         "sig-test",
				InitialState: "Idle",
				States:       core.StateSpecs{{Name: "Idle"}, {Name: "Done"}},
				Signals: core.SignalSpecs{
					{Name: "Seed"},
					{Name: "Orphan"},
				},
				Transitions: []core.TransitionSpec{
					{State: "Idle", Signal: "Seed", Next: "Done"},
				},
				TerminalStates: []string{"Done"},
			},
		},
		MachineOrder: []string{"sig-test"},
	}

	findings := checkMachineSignalCoverage(corpus)
	require.Len(t, findings, 1)
	assert.Contains(t, findings[0].Message, "Orphan")
	assert.Equal(t, "warning", findings[0].Level)
}

func TestValidate_MachineStateMetadata(t *testing.T) {
	corpus := &Corpus{
		Machines: map[string]core.MachineSpec{
			"partial": {
				Name:         "partial",
				InitialState: "Idle",
				States: core.StateSpecs{
					{Name: "Idle", Meaning: "start"},
					{Name: "Done"},
				},
				Signals:        core.SignalSpecs{{Name: "Seed"}},
				Transitions:    []core.TransitionSpec{{State: "Idle", Signal: "Seed", Next: "Done"}},
				TerminalStates: []string{"Done"},
			},
		},
		MachineOrder: []string{"partial"},
	}

	findings := checkMachineStateMetadata(corpus)
	require.Len(t, findings, 1)
	assert.Contains(t, findings[0].Message, "Done")
	assert.Equal(t, "warning", findings[0].Level)
}

func TestDiscoverMachinesIncludesConfiguredToolSelections(t *testing.T) {
	root := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(root, "agents", "critic"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "agents", "critic", "machine.yaml"), []byte(`
name: critic-session
initial_state: Idle
terminal_states: [Done]
configuration:
  point_tools: agents/critic/tools-point.yaml
states:
  - Idle
  - Done
signals:
  - name: Seed
transitions:
  - state: Idle
    signal: Seed
    next: Done
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "agents", "critic", "tools.yaml"), []byte(`
tools:
  - parse_suite_config
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "agents", "critic", "tools-point.yaml"), []byte(`
tools:
  - create_point_dir
  - run_agent
`), 0o644))

	_, selections, _, err := discoverAndParseMachines(root)

	require.NoError(t, err)
	assert.Equal(t, []string{"parse_suite_config"}, selections["critic"])
	assert.Equal(t, []string{"create_point_dir", "run_agent"}, selections["critic:point_tools"])
}

func TestValidate_MachineDiagnostics(t *testing.T) {
	corpus := &Corpus{
		Machines: map[string]core.MachineSpec{
			"diag-test": {
				Name:         "diag-test",
				InitialState: "Idle",
				States:       core.StateSpecs{{Name: "Idle"}, {Name: "Done"}, {Name: "Orphan"}},
				Signals: core.SignalSpecs{
					{Name: "Seed"},
					{Name: "Unused"},
				},
				Transitions: []core.TransitionSpec{
					{State: "Idle", Signal: "Seed", Next: "Done"},
				},
				TerminalStates: []string{"Done"},
			},
		},
		MachineOrder: []string{"diag-test"},
	}
	findings := checkMachineDiagnostics(corpus)
	require.NotEmpty(t, findings)

	var codes []string
	for _, f := range findings {
		codes = append(codes, f.Check)
	}
	assert.Contains(t, codes, "machine-diagnostic-unreachable_state")
	assert.Contains(t, codes, "machine-diagnostic-unused_signal")
}

func TestValidate_MachineDiagnostics_Fixture(t *testing.T) {
	_, c := loadTestGraphAndCorpus(t)
	findings := checkMachineDiagnostics(c)
	assert.Empty(t, findings, "fixture machines should have no diagnostics")
}

func TestValidate_MachineNameConsistency(t *testing.T) {
	corpus := &Corpus{
		Machines: map[string]core.MachineSpec{
			"critic": {
				Name:           "critic-session",
				InitialState:   "Idle",
				States:         core.StateSpecs{{Name: "Idle"}},
				Signals:        core.SignalSpecs{{Name: "Seed"}},
				Transitions:    []core.TransitionSpec{{State: "Idle", Signal: "Seed", Next: "Idle"}},
				TerminalStates: []string{"Idle"},
			},
			"dir-name": {
				Name:           "wrong-name",
				InitialState:   "Idle",
				States:         core.StateSpecs{{Name: "Idle"}},
				Signals:        core.SignalSpecs{{Name: "Seed"}},
				Transitions:    []core.TransitionSpec{{State: "Idle", Signal: "Seed", Next: "Idle"}},
				TerminalStates: []string{"Idle"},
			},
		},
		MachineOrder: []string{"critic", "dir-name"},
	}

	findings := checkMachineNameConsistency(corpus)
	require.Len(t, findings, 1)
	assert.Equal(t, "error", findings[0].Level)
	assert.Contains(t, findings[0].Message, "wrong-name")
}
