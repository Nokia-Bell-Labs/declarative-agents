// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"fmt"
	"time"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
)

// ReportSessionBuilder creates reportSessionCmd instances.
type ReportSessionBuilder struct {
	ES *EvalSessionState
}

func (b *ReportSessionBuilder) Build(_ core.Result) core.Command {
	return &reportSessionCmd{es: b.ES}
}

type reportSessionCmd struct {
	es *EvalSessionState
}

func (c *reportSessionCmd) Name() string      { return "report_session" }
func (c *reportSessionCmd) Undo() core.Result { return core.NoopUndo(c.Name()) }

func (c *reportSessionCmd) Execute() core.Result {
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
