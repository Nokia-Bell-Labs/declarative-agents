// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"fmt"
	"time"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
)

// ReportSessionBuilder creates reportSessionCmd instances.
type ReportSessionBuilder struct {
	ES *EvalSessionState
}

func (b *ReportSessionBuilder) Build(_ core.Result) core.Command {
	return &reportSessionCmd{es: b.ES}
}

type reportSessionCmd struct {
	es          *EvalSessionState
	snapshot    evalSessionSnapshot
	hasSnapshot bool
}

func (c *reportSessionCmd) Name() string { return "report_session" }
func (c *reportSessionCmd) Undo() core.Result {
	return undoEvalSessionSnapshot(c.Name(), c.es, c.snapshot, c.hasSnapshot)
}
func (c *reportSessionCmd) UndoMemento() (core.UndoMemento, error) {
	return evalSessionMemento(c.Name(), c.snapshot, c.hasSnapshot)
}

func (c *reportSessionCmd) Execute() core.Result {
	c.snapshot = snapshotEvalSession(c.es)
	c.hasSnapshot = true
	c.es.FinalizeSession()
	r := &c.es.Result

	fmt.Fprintf(c.es.Stderr, "\nSession complete: %d/%d passed (%d timed out) in %s\n",
		r.Passed, r.TotalPoints, r.TimedOut, r.Duration.Round(time.Second))

	return core.Result{
		Signal:      SigSessionReported,
		Output:      fmt.Sprintf("%d/%d passed", r.Passed, r.TotalPoints),
		CommandName: "report_session",
	}
}

// ReportSessionFactory creates a BuiltinFactory for report_session.
func ReportSessionFactory(es *EvalSessionState) BuiltinFactory {
	return func(def ToolDef, vars map[string]string) (core.Builder, error) {
		return &ReportSessionBuilder{ES: es}, nil
	}
}
