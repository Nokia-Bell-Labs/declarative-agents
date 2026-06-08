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

// Skipped returns the validation tool names that were not dispatched,
// in the fixed order: build, lint, test.
func (t *ToolTracker) Skipped() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	var skipped []string
	for _, name := range []string{"build", "lint", "test"} {
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
}

func (v *validateCmd) Name() string { return "validate" }

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

// ValidateBuilder constructs validate commands.
type ValidateBuilder struct {
	Tracker      *ToolTracker
	BuildBuilder core.Builder
	LintBuilder  core.Builder
	TestBuilder  core.Builder
	Tracer       tracing.Tracer
	Verbose      bool
}

func (b *ValidateBuilder) Build(_ core.Result) core.Command {
	skipped := b.Tracker.Skipped()
	builders := map[string]core.Builder{
		"build": b.BuildBuilder,
		"lint":  b.LintBuilder,
		"test":  b.TestBuilder,
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
