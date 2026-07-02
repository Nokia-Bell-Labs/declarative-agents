// Copyright (c) 2026 Nokia. All rights reserved.

package evaluation

import (
	"fmt"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
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

// ReportSessionFactory creates a registry.BuiltinFactory for report_session.
func ReportSessionFactory(es *EvalSessionState) toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		return &ReportSessionBuilder{ES: es}, nil
	}
}
