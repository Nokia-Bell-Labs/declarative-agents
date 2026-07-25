// Copyright (c) 2026 Nokia. All rights reserved.

package evaluation

import (
	"fmt"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
)

// NextPointBuilder creates nextPointCmd instances.
type NextPointBuilder struct {
	ES *EvalSessionState
}

func (b *NextPointBuilder) Build(_ core.Result) core.Command {
	return &evaluatorReceiptCmd{inner: &nextPointCmd{es: b.ES}, session: b.ES}
}

func (b *NextPointBuilder) BuildReverser() core.Command {
	return &evaluatorReceiptCmd{inner: &nextPointCmd{es: b.ES}, session: b.ES}
}

type nextPointCmd struct {
	es *EvalSessionState
}

func (c *nextPointCmd) Name() string { return "next_point" }
func (c *nextPointCmd) Undo(prior core.Result) core.Result {
	return (&evaluatorReceiptCmd{inner: c, session: c.es}).Undo(prior)
}

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
	_, _ = fmt.Fprintf(c.es.Stderr, "  → %s\n", pc.PointID)

	return core.Result{
		Signal:      SigPointReady,
		Output:      pc.PointID,
		CommandName: "next_point",
	}
}

// NextPointFactory creates a registry.BuiltinFactory for next_point.
func NextPointFactory(es *EvalSessionState) toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		return &NextPointBuilder{ES: es}, nil
	}
}
