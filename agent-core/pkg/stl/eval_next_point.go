// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"fmt"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
)

// NextPointBuilder creates nextPointCmd instances.
type NextPointBuilder struct {
	ES *EvalSessionState
}

func (b *NextPointBuilder) Build(_ core.Result) core.Command {
	return &nextPointCmd{es: b.ES}
}

type nextPointCmd struct {
	es *EvalSessionState
}

func (c *nextPointCmd) Name() string      { return "next_point" }
func (c *nextPointCmd) Undo() core.Result { return core.NoopUndo(c.Name()) }

func (c *nextPointCmd) Execute() core.Result {
	pc, ok := c.es.NextPoint()
	if !ok {
		return core.Result{
			Signal:      SigAllPointsDone,
			Output:      fmt.Sprintf("all points complete: %d/%d passed", c.es.Result.Passed, c.es.Result.TotalPoints),
			CommandName: "next_point",
		}
	}

	c.es.PC = pc
	fmt.Fprintf(c.es.Stderr, "  → %s\n", pc.PointID)

	return core.Result{
		Signal:      SigPointReady,
		Output:      pc.PointID,
		CommandName: "next_point",
	}
}

// NextPointFactory creates a BuiltinFactory for next_point.
func NextPointFactory(es *EvalSessionState) BuiltinFactory {
	return func(def ToolDef, vars map[string]string) (core.Builder, error) {
		return &NextPointBuilder{ES: es}, nil
	}
}
