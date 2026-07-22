// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	"strings"
	"testing"
)

func TestParseMachineSpec_Valid(t *testing.T) {
	t.Parallel()

	spec, err := ParseMachineSpec([]byte(validYAML))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.Name != "test-machine" {
		t.Errorf("name = %q, want %q", spec.Name, "test-machine")
	}
	if spec.InitialState != "Idle" {
		t.Errorf("initial_state = %q, want %q", spec.InitialState, "Idle")
	}
	if len(spec.States) != 4 {
		t.Errorf("states count = %d, want 4", len(spec.States))
	}
	if len(spec.TerminalStates) != 2 {
		t.Errorf("terminal_states count = %d, want 2", len(spec.TerminalStates))
	}
	if len(spec.Transitions) != 3 {
		t.Errorf("transitions count = %d, want 3", len(spec.Transitions))
	}
}

func TestParseMachineSpecRejectsDuplicateGrammarEntries(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{
			name:    "state name",
			input:   strings.Replace(validYAML, "states: [Idle, Running, Done, Error]", "states: [Idle, Running, Done, Error, Running]", 1),
			wantErr: `states[4]: duplicate name "Running"`,
		},
		{
			name:    "terminal state",
			input:   strings.Replace(validYAML, "terminal_states: [Done, Error]", "terminal_states: [Done, Error, Done]", 1),
			wantErr: `terminal_states[2]: duplicate state "Done"`,
		},
		{
			name:    "signal name",
			input:   strings.Replace(validYAML, "signals: [Start, Finished, Failed]", "signals: [Start, Finished, Failed, Start]", 1),
			wantErr: `signals[3]: duplicate name "Start"`,
		},
		{
			name: "transition key",
			input: validYAML + `
  - state: Idle
    signal: Start
    next: Error
`,
			wantErr: `transitions[3]: duplicate state "Idle" and signal "Start"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := ParseMachineSpec([]byte(tt.input))
			require.ErrorContains(t, err, tt.wantErr)
			assert.ErrorContains(t, err, "first declared at")
		})
	}
}

func TestMachineSpecTransitionTableRoundTripPreservesSemantics(t *testing.T) {
	t.Parallel()
	spec, err := ParseMachineSpec([]byte(validYAML))
	require.NoError(t, err)
	encoded, err := yaml.Marshal(spec)
	require.NoError(t, err)
	roundTrip, err := ParseMachineSpec(encoded)
	require.NoError(t, err)

	registry := NewRegistry()
	registry.Register(ToolSpec{Name: "do_work"}, &dummyBuilder{name: "do_work"})
	originalTable, _, err := BuildTransitionTable(spec, registry, nil)
	require.NoError(t, err)
	roundTripTable, _, err := BuildTransitionTable(roundTrip, registry, nil)
	require.NoError(t, err)

	require.Len(t, originalTable, len(spec.Transitions))
	require.Len(t, roundTripTable, len(roundTrip.Transitions))
	require.Equal(t, len(originalTable), len(roundTripTable))
	for key, original := range originalTable {
		restored, exists := roundTripTable[key]
		require.True(t, exists, "missing transition %#v after round trip", key)
		assert.Equal(t, original.NextState, restored.NextState)
		assert.Equal(t, original.Action == nil, restored.Action == nil)
	}
}

func TestParseMachineSpec_RichStateAndSignalMetadata(t *testing.T) {
	t.Parallel()

	yaml := `
name: rich
purpose: Coordinate a review workflow.
invariants:
  - terminal states do not dispatch commands
lifecycle: approval-gated
configuration:
  owner: qa
pipeline_diagram: Start -> Working -> Done
initial_state: Start
states:
  - name: Start
    meaning: Ready to dispatch first command.
  - Working
  - name: Done
    meaning: Terminal success state.
terminal_states: [Done]
signals:
  - name: Seed
    trigger: Engine bootstrap.
  - ToolDone
transitions:
  - state: Start
    signal: Seed
    next: Working
    action: step
  - state: Working
    signal: ToolDone
    next: Done
`
	spec, err := ParseMachineSpec([]byte(yaml))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if spec.Purpose != "Coordinate a review workflow." {
		t.Fatalf("purpose = %q", spec.Purpose)
	}
	if len(spec.Invariants) != 1 {
		t.Fatalf("invariants = %#v", spec.Invariants)
	}
	if spec.Lifecycle != "approval-gated" {
		t.Fatalf("lifecycle = %q", spec.Lifecycle)
	}
	if spec.Configuration["owner"] != "qa" {
		t.Fatalf("configuration = %#v", spec.Configuration)
	}
	if spec.PipelineDiagram == "" {
		t.Fatal("pipeline_diagram should parse")
	}
	if got := spec.States.Names(); strings.Join(got, ",") != "Start,Working,Done" {
		t.Fatalf("state names = %#v", got)
	}
	if spec.States[0].Meaning == "" || spec.States[2].Meaning == "" {
		t.Fatalf("state metadata not preserved: %#v", spec.States)
	}
	if got := spec.Signals.Names(); strings.Join(got, ",") != "Seed,ToolDone" {
		t.Fatalf("signal names = %#v", got)
	}
	if spec.Signals[0].Trigger == "" {
		t.Fatalf("signal metadata not preserved: %#v", spec.Signals)
	}

	reg := NewRegistry()
	reg.Register(ToolSpec{Name: "step"}, &dummyBuilder{name: "step"})
	_, isTerminal, err := BuildTransitionTable(spec, reg, nil)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !isTerminal("Done") {
		t.Fatal("Done should be terminal")
	}

	data, err := MarshalMachineSpec(spec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	reparsed, err := ParseMachineSpec(data)
	if err != nil {
		t.Fatalf("reparse: %v\n%s", err, data)
	}
	if reparsed.Purpose != spec.Purpose || reparsed.Lifecycle != spec.Lifecycle || len(reparsed.Invariants) != len(spec.Invariants) {
		t.Fatalf("metadata did not round-trip: %#v", reparsed)
	}
}

func TestParseMachineSpec_MetricLabels(t *testing.T) {
	t.Parallel()
	yaml := `
name: metric-machine
initial_state: Idle
metric_labels:
  use_case: rel04.0-monitor
  phase: setup
states: [Idle, Running, Done]
terminal_states: [Done]
signals: [Seed, ToolDone]
transitions:
  - state: Idle
    signal: Seed
    next: Running
    action: run
    metric_labels:
      phase: main_loop
  - state: Running
    signal: ToolDone
    next: Done
`
	spec, err := ParseMachineSpec([]byte(yaml))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if spec.MetricLabels["phase"] != "setup" {
		t.Fatalf("machine metric labels = %#v", spec.MetricLabels)
	}
	if spec.Transitions[0].MetricLabels["phase"] != "main_loop" {
		t.Fatalf("transition metric labels = %#v", spec.Transitions[0].MetricLabels)
	}
}

func TestParseMachineSpec_MetricLabelsRejectUnsafeValues(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		yaml string
		want string
	}{
		{name: "invalid machine label name", yaml: metricLabelYAML("metric_labels:\n  9bad: value"), want: "not a valid metric name"},
		{name: "unsafe machine label value", yaml: metricLabelYAML("metric_labels:\n  phase: raw_prompt"), want: "not a safe metric label"},
		{name: "unsafe transition label value", yaml: transitionMetricLabelYAML("phase: arbitrary_url"), want: "not a safe metric label"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := ParseMachineSpec([]byte(tc.yaml))
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q missing %q", err.Error(), tc.want)
			}
		})
	}
}

func TestParseMachineSpecValidationErrors(t *testing.T) {
	t.Parallel()
	richConflicts := `
name: rich-bad
initial_state: Start
states:
  - name: Start
  - {}
terminal_states: [Missing]
signals:
  - name: Seed
  - {}
transitions:
  - state: Start
    signal: MissingSignal
    next: Missing
`
	tests := []struct {
		name  string
		yaml  string
		wants []string
	}{
		{name: "missing initial state", yaml: strings.Replace(validYAML, "initial_state: Idle\n", "", 1), wants: []string{"initial_state is required"}},
		{name: "unknown transition state", yaml: strings.Replace(validYAML, "state: Idle", "state: Missing", 1), wants: []string{`state "Missing" not in states list`}},
		{name: "unknown transition signal", yaml: strings.Replace(validYAML, "signal: Start", "signal: Missing", 1), wants: []string{`signal "Missing" not in signals list`}},
		{name: "unknown terminal state", yaml: strings.Replace(validYAML, "terminal_states: [Done, Error]", "terminal_states: [Done, Missing]", 1), wants: []string{`terminal_state "Missing" not in states list`}},
		{
			name: "rich names and conflicting references", yaml: richConflicts,
			wants: []string{
				"states[1]: name is required",
				"signals[1]: name is required",
				`terminal_state "Missing" not in states list`,
				`signal "MissingSignal" not in signals list`,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := ParseMachineSpec([]byte(tt.yaml))
			require.Error(t, err)
			for _, want := range tt.wants {
				assert.ErrorContains(t, err, want)
			}
		})
	}
}

func TestLoadMachineSpec_FileNotFound(t *testing.T) {
	t.Parallel()

	_, err := LoadMachineSpec("/nonexistent/path/machine.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}
