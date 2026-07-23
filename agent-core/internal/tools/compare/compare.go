// Copyright (c) 2026 Nokia. All rights reserved.

// Package compare provides the compare_state builtin: a word that compares two
// command-state $from(label).path values and emits one of two configured
// signals, so a machine can branch on a value a prior step reported without
// that comparison being written in Go (srd038-command-state-store).
package compare

import (
	"encoding/json"
	"fmt"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

const (
	defaultMatchedSignal  = core.Signal("ValuesMatched")
	defaultDifferedSignal = core.Signal("ValuesDiffered")
)

// Builder constructs compare_state commands from two $from selectors and the
// signals to emit for an equal and an unequal verdict.
type Builder struct {
	ToolName string
	Left     string
	Right    string
	Matched  core.Signal
	Differed core.Signal
}

// Build returns a compare_state command. The engine injects the command-state
// view before dispatch (core.CommandStateAware).
func (b Builder) Build(_ core.Result) core.Command {
	return &compareCmd{
		name:     b.ToolName,
		left:     b.Left,
		right:    b.Right,
		matched:  b.Matched,
		differed: b.Differed,
	}
}

type compareCmd struct {
	name     string
	left     string
	right    string
	matched  core.Signal
	differed core.Signal
	view     core.CommandStateView
}

func (c *compareCmd) Name() string { return c.name }

// SetCommandState receives the read-only command-state view so both operands
// resolve against prior steps.
func (c *compareCmd) SetCommandState(view core.CommandStateView) { c.view = view }

var _ core.CommandStateAware = (*compareCmd)(nil)

func (c *compareCmd) Undo(_ core.Result) core.Result { return core.NoopUndo(c.Name()) }

// Execute resolves both selectors and emits the matched or differed signal.
//
// An unresolved selector emits CommandError rather than a verdict. This departs
// from compose, where an unresolved selector renders empty so a degraded
// upstream step still yields a prompt: a comparison whose operand is missing
// has no defensible verdict, and a caller must be able to tell "these differ"
// from "one side is unknown" (srd038-command-state-store R1.5).
func (c *compareCmd) Execute() core.Result {
	left, err := c.resolve(c.left, "left")
	if err != nil {
		return core.Result{Signal: core.CommandError, CommandName: c.Name(), Err: err}
	}
	right, err := c.resolve(c.right, "right")
	if err != nil {
		return core.Result{Signal: core.CommandError, CommandName: c.Name(), Err: err}
	}

	signal := c.differedSignal()
	verdict := "differed"
	if canonical(left) == canonical(right) {
		signal = c.matchedSignal()
		verdict = "matched"
	}

	return core.Result{
		Signal:      signal,
		CommandName: c.Name(),
		Output:      verdictOutput(verdict, canonical(left), canonical(right)),
	}
}

// verdictOutput renders the comparison as a JSON object string. The signal is
// what a machine branches on; this records which values produced that verdict
// so a trace shows why a source was excluded rather than only that it was.
func verdictOutput(verdict, left, right string) string {
	data, err := json.Marshal(map[string]string{
		"verdict": verdict,
		"left":    left,
		"right":   right,
	})
	if err != nil {
		return verdict
	}
	return string(data)
}

func (c *compareCmd) resolve(selector, side string) (any, error) {
	if selector == "" {
		return nil, fmt.Errorf("compare_state: %s selector is empty", side)
	}
	value, err := core.ResolveFromSelector(c.view, selector)
	if err != nil {
		return nil, fmt.Errorf("compare_state: %s selector %s: %w", side, selector, err)
	}
	return value, nil
}

func (c *compareCmd) matchedSignal() core.Signal {
	if c.matched == "" {
		return defaultMatchedSignal
	}
	return c.matched
}

func (c *compareCmd) differedSignal() core.Signal {
	if c.differed == "" {
		return defaultDifferedSignal
	}
	return c.differed
}

// canonical renders a resolved value for comparison: strings compare directly,
// anything else compares by its JSON encoding, so structured values compare by
// content rather than by Go identity.
func canonical(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(data)
}
