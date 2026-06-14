// Copyright (c) 2026 Nokia. All rights reserved.

package evaluation

import (
	"fmt"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/catalog"
	toolregistry "gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/registry"
)

// NextPointBuilder creates nextPointCmd instances.
type NextPointBuilder struct {
	ES *EvalSessionState
}

func (b *NextPointBuilder) Build(_ core.Result) core.Command {
	return &nextPointCmd{es: b.ES}
}

type nextPointCmd struct {
	es          *EvalSessionState
	snapshot    evalSessionSnapshot
	hasSnapshot bool
}

func (c *nextPointCmd) Name() string { return "next_point" }
func (c *nextPointCmd) Undo() core.Result {
	return undoEvalSessionSnapshot(c.Name(), c.es, c.snapshot, c.hasSnapshot)
}
func (c *nextPointCmd) UndoMemento() (core.UndoMemento, error) {
	return evalSessionMemento(c.Name(), c.snapshot, c.hasSnapshot)
}

func (c *nextPointCmd) Execute() core.Result {
	c.snapshot = snapshotEvalSession(c.es)
	c.hasSnapshot = true
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

// NextPointFactory creates a registry.BuiltinFactory for next_point.
func NextPointFactory(es *EvalSessionState) toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		return &NextPointBuilder{ES: es}, nil
	}
}
