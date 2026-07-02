// Copyright (c) 2026 Nokia. All rights reserved.

package validation

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"

	modelllm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/undo"
)

// ToolTracker records which external tool names have been dispatched.
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

// Skipped returns validation tool names that were not dispatched.
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

// Reset clears the tracker for the next pass.
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
	return undo.BoundaryCompensationUndo(v.Name(), "undo or compensate validation child commands and any workspace effects they produced")
}
func (v *validateCmd) UndoMemento() (core.UndoMemento, error) {
	payload := undo.BoundaryCompensationPayload{BoundaryCompensation: undo.BoundaryCompensation{
		Strategy: "child_command_undo",
		Reason:   "aggregate validation replays child tools",
		Requires: append([]string(nil), v.ran...),
	}}
	return undo.BoundaryCompensationMemento(v.Name(), payload, "undo is delegated to the validation child commands captured in the payload")
}

func (v *validateCmd) Execute() core.Result {
	if len(v.skipped) == 0 {
		if v.tracer != nil {
			v.tracer.Event("validation.none_skipped")
		}
		return core.Result{Output: "validation passed: no tools were skipped", Signal: core.ValidationPassed, CommandName: "validate"}
	}
	start := time.Now()
	var totalCost core.Cost
	var ran []string
	v.ran = nil
	for _, toolName := range v.skipped {
		res, ok := v.runChild(toolName)
		if !ok {
			continue
		}
		totalCost.Duration += res.Cost.Duration
		ran = append(ran, toolName)
		v.ran = append(v.ran, toolName)
		if result, done := v.childResult(toolName, res, totalCost); done {
			return result
		}
	}
	totalCost.Duration = time.Since(start)
	return core.Result{Output: fmt.Sprintf("validation passed: ran %s", strings.Join(ran, ", ")), Signal: core.ValidationPassed, Cost: totalCost, CommandName: "validate"}
}

func (v *validateCmd) runChild(toolName string) (core.Result, bool) {
	builder, ok := v.builders[toolName]
	if !ok {
		return core.Result{}, false
	}
	childTrace, childDone := v.startChildTrace(toolName)
	cmd := builder.Build(core.Result{})
	res := cmd.Execute()
	v.finishChildTrace(childTrace, childDone, res)
	return res, true
}

func (v *validateCmd) startChildTrace(toolName string) (tracing.Tracer, func()) {
	if v.tracer == nil {
		return nil, nil
	}
	return v.tracer.Push("validate/"+toolName, attribute.String("tool_name", toolName))
}

func (v *validateCmd) finishChildTrace(childTrace tracing.Tracer, childDone func(), res core.Result) {
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
}

func (v *validateCmd) childResult(toolName string, res core.Result, cost core.Cost) (core.Result, bool) {
	if res.Signal == core.CommandError {
		return core.Result{Output: fmt.Sprintf("validation infrastructure error in %s: %s", toolName, res.Output), Signal: core.CommandError, Err: res.Err, Cost: cost, CommandName: "validate"}, true
	}
	if res.Signal == core.ToolFailed {
		if v.tracer != nil {
			v.tracer.Event("validation.failed", attribute.String("failing_tool", toolName), attribute.String("output_summary", modelllm.Truncate(res.Output, 200)))
		}
		return core.Result{Output: fmt.Sprintf("validation failed at %s: %s", toolName, res.Output), Signal: core.ValidationFailed, Cost: cost, CommandName: "validate"}, true
	}
	return core.Result{}, false
}

// ValidateBuilder constructs validate commands.
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
	return &validateCmd{skipped: skipped, builders: builders, tracer: b.Tracer, verbose: b.Verbose}
}

// ValidateToolSpec returns the ToolSpec for the validate internal command.
func ValidateToolSpec() core.ToolSpec {
	return core.ToolSpec{Name: "validate", Visibility: core.Internal}
}
