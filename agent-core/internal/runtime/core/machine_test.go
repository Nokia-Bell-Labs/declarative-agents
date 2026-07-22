// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const validYAML = `
name: test-machine
initial_state: Idle
states: [Idle, Running, Done, Error]
terminal_states: [Done, Error]
signals: [Start, Finished, Failed]
transitions:
  - state: Idle
    signal: Start
    next: Running
    action: do_work
  - state: Running
    signal: Finished
    next: Done
  - state: Running
    signal: Failed
    next: Error
`

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

func metricLabelYAML(labels string) string {
	return strings.Replace(validYAML, "name: test-machine", "name: test-machine\n"+labels, 1)
}

func transitionMetricLabelYAML(label string) string {
	return strings.Replace(validYAML, "    action: do_work", "    action: do_work\n    metric_labels:\n      "+label, 1)
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

func TestDiagnoseMachineSpecCleanMachine(t *testing.T) {
	t.Parallel()

	spec, err := ParseMachineSpec([]byte(validYAML))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	diagnostics := DiagnoseMachineSpec(spec)

	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diagnostics)
	}
}

func TestDiagnoseMachineSpecReportsReachabilityAndDeadGrammar(t *testing.T) {
	t.Parallel()

	yaml := `
name: diagnostics
initial_state: Start
states: [Start, Working, Done, Orphan, TerminalWithTransition]
terminal_states: [Done, TerminalWithTransition]
signals: [Seed, ToolDone, Unused]
transitions:
  - state: Start
    signal: Seed
    next: Working
    action: step
  - state: Working
    signal: ToolDone
    next: Done
  - state: Orphan
    signal: ToolDone
    next: Done
  - state: TerminalWithTransition
    signal: ToolDone
    next: Done
`
	spec, err := ParseMachineSpec([]byte(yaml))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	diagnostics := DiagnoseMachineSpec(spec)

	assertMachineDiagnostic(t, diagnostics, "unreachable_state", "Orphan", "", 0)
	assertMachineDiagnostic(t, diagnostics, "unreachable_transition", "Orphan", "ToolDone", 2)
	assertMachineDiagnostic(t, diagnostics, "terminal_transition", "TerminalWithTransition", "ToolDone", 3)
	assertMachineDiagnostic(t, diagnostics, "unused_signal", "", "Unused", 0)
}

func TestDiagnoseMachineSpecUsesRichNames(t *testing.T) {
	t.Parallel()

	yaml := `
name: diagnostics-rich
initial_state: Start
states:
  - name: Start
    meaning: Entry point.
  - name: Done
    meaning: Terminal state.
  - name: Orphan
    meaning: Unreachable branch.
terminal_states: [Done]
signals:
  - name: Seed
    trigger: Engine bootstrap.
  - name: Unused
    trigger: Never emitted.
transitions:
  - state: Start
    signal: Seed
    next: Done
`
	spec, err := ParseMachineSpec([]byte(yaml))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	diagnostics := DiagnoseMachineSpec(spec)

	assertMachineDiagnostic(t, diagnostics, "unreachable_state", "Orphan", "", 0)
	assertMachineDiagnostic(t, diagnostics, "unused_signal", "", "Unused", 0)
}

type dummyCmd struct{ name string }

func (d *dummyCmd) Name() string         { return d.name }
func (d *dummyCmd) Execute() Result      { return Result{} }
func (d *dummyCmd) Undo(_ Result) Result { return NoopUndo(d.name) }

type dummyBuilder struct{ name string }

func (d *dummyBuilder) Build(_ Result) Command { return &dummyCmd{name: d.name} }

func TestBuildTransitionTable(t *testing.T) {
	t.Parallel()

	spec, err := ParseMachineSpec([]byte(validYAML))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	reg := NewRegistry()
	reg.Register(ToolSpec{Name: "do_work", Phases: []State{"Running"}}, &dummyBuilder{name: "do_work"})

	table, isTerminal, err := BuildTransitionTable(spec, reg, nil)
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	if len(table) != 3 {
		t.Errorf("table size = %d, want 3", len(table))
	}

	if !isTerminal("Done") {
		t.Error("Done should be terminal")
	}
	if !isTerminal("Error") {
		t.Error("Error should be terminal")
	}
	if isTerminal("Idle") {
		t.Error("Idle should not be terminal")
	}

	key := TransitionInput{State: "Idle", Signal: "Start"}
	tv, ok := table[key]
	if !ok {
		t.Fatal("missing transition for Idle+Start")
	}
	if tv.NextState != "Running" {
		t.Errorf("next = %q, want Running", tv.NextState)
	}
	if tv.Action == nil {
		t.Error("action should not be nil for do_work transition")
	}

	key2 := TransitionInput{State: "Running", Signal: "Finished"}
	tv2, ok := table[key2]
	if !ok {
		t.Fatal("missing transition for Running+Finished")
	}
	if tv2.Action != nil {
		t.Error("terminal transition action should be nil")
	}
}

func TestBuildTransitionTable_ToolActionSentinel(t *testing.T) {
	t.Parallel()

	yaml := `
name: tool-test
initial_state: A
states: [A, B]
terminal_states: [B]
signals: [Go]
transitions:
  - state: A
    signal: Go
    next: B
    action: $tool
`
	spec, err := ParseMachineSpec([]byte(yaml))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	called := false
	toolAction := func(r Result) Command {
		called = true
		return &dummyCmd{}
	}

	reg := NewRegistry()
	table, _, err := BuildTransitionTable(spec, reg, toolAction)
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	key := TransitionInput{State: "A", Signal: "Go"}
	tv := table[key]
	tv.Action(Result{})
	if !called {
		t.Error("$tool action should have invoked the toolAction function")
	}
}

func TestBuildTransitionTablePassesTargetStateToAction(t *testing.T) {
	t.Parallel()

	yaml := `
name: state-context
initial_state: A
states: [A, B]
terminal_states: [B]
signals: [Go]
transitions:
  - state: A
    signal: Go
    next: B
    action: $tool
`
	spec, err := ParseMachineSpec([]byte(yaml))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	var got State
	toolAction := func(r Result) Command {
		got = r.State
		return &dummyCmd{}
	}

	reg := NewRegistry()
	table, _, err := BuildTransitionTable(spec, reg, toolAction)
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	table[TransitionInput{State: "A", Signal: "Go"}].Action(Result{})
	if got != "B" {
		t.Fatalf("action state = %q, want B", got)
	}
}

func TestBuildTransitionTable_ToolActionNil(t *testing.T) {
	t.Parallel()

	yaml := `
name: tool-test
initial_state: A
states: [A, B]
terminal_states: [B]
signals: [Go]
transitions:
  - state: A
    signal: Go
    next: B
    action: $tool
`
	spec, err := ParseMachineSpec([]byte(yaml))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	reg := NewRegistry()
	_, _, err = BuildTransitionTable(spec, reg, nil)
	if err == nil {
		t.Fatal("expected error when $tool action used without toolAction function")
	}
}

func TestBuildTransitionTable_UnknownAction(t *testing.T) {
	t.Parallel()

	yaml := `
name: bad-action
initial_state: A
states: [A, B]
terminal_states: [B]
signals: [Go]
transitions:
  - state: A
    signal: Go
    next: B
    action: nonexistent
`
	spec, err := ParseMachineSpec([]byte(yaml))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	reg := NewRegistry()
	_, _, err = BuildTransitionTable(spec, reg, nil)
	if err == nil {
		t.Fatal("expected error for unknown action")
	}
	if !strings.Contains(err.Error(), `"nonexistent" not found in registry`) {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLoadMachineSpec_FileNotFound(t *testing.T) {
	t.Parallel()

	_, err := LoadMachineSpec("/nonexistent/path/machine.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

// TestToolComposition_ViaStateMachine demonstrates the pattern of composing
// atomic tools via machine.yaml transitions. Three tools (stage, commit,
// tag) are chained through state transitions, each emitting ToolDone to
// advance to the next step. The state machine is the composition layer.
func TestToolComposition_ViaStateMachine(t *testing.T) {
	t.Parallel()

	machineYAML := `
name: commit-workflow
initial_state: Idle
states: [Idle, Staging, Committing, Tagging, Done, Failed]
terminal_states: [Done, Failed]
signals: [Seed, ToolDone, ToolFailed, CommandError]
transitions:
  - state: Idle
    signal: Seed
    next: Staging
    action: stage_all
  - state: Staging
    signal: ToolDone
    next: Committing
    action: commit
  - state: Staging
    signal: ToolFailed
    next: Failed
  - state: Committing
    signal: ToolDone
    next: Tagging
    action: tag
  - state: Committing
    signal: ToolFailed
    next: Failed
  - state: Tagging
    signal: ToolDone
    next: Done
  - state: Tagging
    signal: ToolFailed
    next: Failed
`
	spec, err := ParseMachineSpec([]byte(machineYAML))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	var executionOrder []string
	makeBuilder := func(name string) Builder {
		return &orderTracker{toolName: name, order: &executionOrder}
	}

	reg := NewRegistry()
	reg.Register(ToolSpec{Name: "stage_all"}, makeBuilder("stage_all"))
	reg.Register(ToolSpec{Name: "commit"}, makeBuilder("commit"))
	reg.Register(ToolSpec{Name: "tag"}, makeBuilder("tag"))

	table, isTerminal, err := BuildTransitionTable(spec, reg, nil)
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	sm := NewStateMachine(table, isTerminal)
	state := State("Idle")
	result := Result{Signal: Seed}

	for !sm.IsTerminal(state) {
		nextState, cmd, err := sm.Step(state, result.Signal, result)
		if err != nil {
			t.Fatalf("step from %s with %s: %v", state, result.Signal, err)
		}
		state = nextState
		if cmd != nil {
			result = cmd.Execute()
		}
	}

	if state != "Done" {
		t.Errorf("final state = %q, want Done", state)
	}

	expected := []string{"stage_all", "commit", "tag"}
	if len(executionOrder) != len(expected) {
		t.Fatalf("execution order length = %d, want %d: %v", len(executionOrder), len(expected), executionOrder)
	}
	for i, name := range expected {
		if executionOrder[i] != name {
			t.Errorf("execution order[%d] = %q, want %q", i, executionOrder[i], name)
		}
	}
}

type orderTracker struct {
	toolName string
	order    *[]string
}

func (o *orderTracker) Build(_ Result) Command {
	return &orderCmd{toolName: o.toolName, order: o.order}
}

type orderCmd struct {
	toolName string
	order    *[]string
}

func (o *orderCmd) Name() string { return o.toolName }
func (o *orderCmd) Execute() Result {
	*o.order = append(*o.order, o.toolName)
	return Result{Signal: ToolDone, CommandName: o.toolName, Output: o.toolName + " done"}
}
func (o *orderCmd) Undo(_ Result) Result { return NoopUndo(o.toolName) }

func assertMachineDiagnostic(t *testing.T, diagnostics []MachineDiagnostic, code, state, signal string, transitionIndex int) {
	t.Helper()
	for _, diagnostic := range diagnostics {
		if diagnostic.Code != code {
			continue
		}
		if state != "" && diagnostic.State != state {
			continue
		}
		if signal != "" && diagnostic.Signal != signal {
			continue
		}
		if transitionIndex != 0 && diagnostic.TransitionIndex != transitionIndex {
			continue
		}
		if diagnostic.Severity != MachineDiagnosticWarning {
			t.Fatalf("diagnostic severity = %q, want %q", diagnostic.Severity, MachineDiagnosticWarning)
		}
		if diagnostic.Message == "" {
			t.Fatal("diagnostic message should not be empty")
		}
		return
	}
	t.Fatalf("missing diagnostic code=%s state=%s signal=%s transition=%d in %#v", code, state, signal, transitionIndex, diagnostics)
}
