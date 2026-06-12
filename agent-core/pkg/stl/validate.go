// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/llm"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/tracing"
)

// ToolTracker records which external tool names have been dispatched
// during the current agentic pass.
type ToolTracker struct {
	mu       sync.Mutex
	recorded map[string]bool
}

// NewToolTracker creates an empty tracker.
func NewToolTracker() *ToolTracker {
	return &ToolTracker{recorded: make(map[string]bool)}
}

// Record marks a tool name as dispatched.
func (t *ToolTracker) Record(name string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.recorded[name] = true
}

// Skipped returns the provided validation tool names that were not dispatched,
// preserving the order from the caller's program specification.
func (t *ToolTracker) Skipped(order []string) []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	var skipped []string
	for _, name := range order {
		if !t.recorded[name] {
			skipped = append(skipped, name)
		}
	}
	return skipped
}

// Reset clears the tracker for the next agentic pass.
func (t *ToolTracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.recorded = make(map[string]bool)
}

type validateCmd struct {
	skipped  []string
	builders map[string]core.Builder
	tracer   tracing.Tracer
	verbose  bool
	ran      []string
}

func (v *validateCmd) Name() string { return "validate" }
func (v *validateCmd) Undo() core.Result {
	return boundaryCompensationUndo(v.Name(), "undo or compensate validation child commands and any workspace effects they produced")
}
func (v *validateCmd) UndoMemento() (core.UndoMemento, error) {
	return boundaryCompensationMemento(v.Name(), BoundaryCompensationPayload{
		BoundaryCompensation: BoundaryCompensation{
			Strategy: "child_command_undo",
			Reason:   "aggregate validation replays child tools",
			Requires: append([]string(nil), v.ran...),
		},
	}, "undo is delegated to the validation child commands captured in the payload")
}

func (v *validateCmd) Execute() core.Result {
	if len(v.skipped) == 0 {
		if v.tracer != nil {
			v.tracer.Event("validation.none_skipped")
		}
		return core.Result{
			Output:      "validation passed: no tools were skipped",
			Signal:      core.ValidationPassed,
			CommandName: "validate",
		}
	}

	start := time.Now()
	var totalCost core.Cost
	var ran []string
	v.ran = nil

	for _, toolName := range v.skipped {
		builder, ok := v.builders[toolName]
		if !ok {
			continue
		}

		var childTrace tracing.Tracer
		var childDone func()
		if v.tracer != nil {
			childTrace, childDone = v.tracer.Push("validate/"+toolName,
				attribute.String("tool_name", toolName),
			)
		}

		cmd := builder.Build(core.Result{})
		res := cmd.Execute()

		if childTrace != nil {
			childTrace.SetAttributes(
				attribute.String("Signal", string(res.Signal)),
				attribute.Int64("Duration", res.Cost.Duration.Milliseconds()),
			)
			if v.verbose {
				childTrace.SetAttributes(attribute.String("tool.output", res.Output))
			}
		}
		if childDone != nil {
			childDone()
		}

		totalCost.Duration += res.Cost.Duration
		ran = append(ran, toolName)
		v.ran = append(v.ran, toolName)

		if res.Signal == core.CommandError {
			return core.Result{
				Output:      fmt.Sprintf("validation infrastructure error in %s: %s", toolName, res.Output),
				Signal:      core.CommandError,
				Err:         res.Err,
				Cost:        totalCost,
				CommandName: "validate",
			}
		}

		if res.Signal == core.ToolFailed {
			if v.tracer != nil {
				v.tracer.Event("validation.failed",
					attribute.String("failing_tool", toolName),
					attribute.String("output_summary", llm.Truncate(res.Output, 200)),
				)
			}
			return core.Result{
				Output:      fmt.Sprintf("validation failed at %s: %s", toolName, res.Output),
				Signal:      core.ValidationFailed,
				Cost:        totalCost,
				CommandName: "validate",
			}
		}
	}

	totalCost.Duration = time.Since(start)

	if v.tracer != nil {
		v.tracer.Event("validation.passed",
			attribute.StringSlice("tools_ran", ran),
		)
	}

	return core.Result{
		Output:      fmt.Sprintf("validation passed: ran %s", strings.Join(ran, ", ")),
		Signal:      core.ValidationPassed,
		Cost:        totalCost,
		CommandName: "validate",
	}
}

// ValidateBuilder constructs validate commands. The validation sequence is
// supplied by the program specification; the builder resolves those named tools
// from the registry at Build time.
type ValidateBuilder struct {
	Tracker  *ToolTracker
	Registry *core.Registry
	Tracer   tracing.Tracer
	Verbose  bool
	Tools    []string
}

func (b *ValidateBuilder) Build(_ core.Result) core.Command {
	skipped := b.Tracker.Skipped(b.Tools)
	builders := make(map[string]core.Builder, len(skipped))
	for _, name := range skipped {
		if builder, ok := b.Registry.Resolve(name); ok {
			builders[name] = builder
		}
	}
	return &validateCmd{
		skipped:  skipped,
		builders: builders,
		tracer:   b.Tracer,
		verbose:  b.Verbose,
	}
}

// ValidateToolSpec returns the ToolSpec for the validate internal command.
func ValidateToolSpec() core.ToolSpec {
	return core.ToolSpec{
		Name:       "validate",
		Visibility: core.Internal,
	}
}
