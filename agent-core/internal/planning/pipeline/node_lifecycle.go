// Copyright (c) 2026 Nokia. All rights reserved.

package pipeline

import (
	"fmt"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/planning/graph"
)

// advanceTaskNodesTo advances every node of the current task to the target
// graph status, owning the Pending -> Planning -> Executing -> Done lifecycle
// across the extract, execute, and check phases. Without these transitions a
// completed node stays Pending, so Graph.Ready keeps returning it and the
// planner re-selects the same work until the budget is exhausted (GH-507).
//
// Each step is idempotent: a node already at (or advanced past, on a retry
// re-execution) the target is a no-op, but a genuinely invalid transition is
// returned so callers can propagate it rather than discard it.
func (ps *State) advanceTaskNodesTo(target graph.Status) error {
	if ps.CurrentTask == nil || ps.Graph == nil {
		return nil
	}
	for _, nid := range ps.CurrentTask.NodeIDs {
		n, ok := ps.Graph.Node(nid)
		if !ok || n == nil {
			continue
		}
		if err := advanceNode(n, target); err != nil {
			return fmt.Errorf("node %s: %w", nid, err)
		}
	}
	return nil
}

// advanceNode moves a single node one step toward target, or no-ops when it is
// already there. It only performs the immediately-prior transition, so an
// out-of-order request (for example Done from Pending) is reported as an error.
func advanceNode(n *graph.Node, target graph.Status) error {
	if n.Status == target {
		return nil
	}
	switch target {
	case graph.Planning:
		if n.Status == graph.Pending {
			return n.MarkPlanning()
		}
	case graph.Executing:
		if n.Status == graph.Executing {
			return nil
		}
		if n.Status == graph.Planning {
			return n.MarkExecuting()
		}
	case graph.Done:
		if n.Status == graph.Executing {
			return n.MarkDone()
		}
	}
	return fmt.Errorf("cannot advance node %s from %s to %s", n.ID, n.Status, target)
}
