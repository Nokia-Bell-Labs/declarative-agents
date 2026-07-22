// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"strings"
	"testing"
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

func metricLabelYAML(labels string) string {
	return strings.Replace(validYAML, "name: test-machine", "name: test-machine\n"+labels, 1)
}

func transitionMetricLabelYAML(label string) string {
	return strings.Replace(validYAML, "    action: do_work", "    action: do_work\n    metric_labels:\n      "+label, 1)
}

type dummyCmd struct{ name string }

func (d *dummyCmd) Name() string { return d.name }

func (d *dummyCmd) Execute() Result { return Result{} }

func (d *dummyCmd) Undo(_ Result) Result { return NoopUndo(d.name) }

type dummyBuilder struct{ name string }

func (d *dummyBuilder) Build(_ Result) Command { return &dummyCmd{name: d.name} }

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
