// Copyright (c) 2026 Nokia. All rights reserved.

// Package core provides the generic agentic loop engine: a state machine,
// command dispatch, tool registry, budget tracking, and tracing.
package core

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/monitor"
)

// State represents a position in the agentic loop lifecycle.
type State string

// Signal carries the outcome of a Command back to the state machine.
type Signal string

// Generic signals used by the loop engine itself.
const (
	Seed            Signal = "Seed"
	BudgetExhausted Signal = "BudgetExhausted"
	CommandError    Signal = "CommandError"
)

// Standard tool signals used by the STL and available to all agents.
const (
	ToolDone   Signal = "ToolDone"
	ToolFailed Signal = "ToolFailed"
	EditDone   Signal = "EditDone"
)

// LLM tool signals used by the STL invoke/parse commands.
const (
	LLMResponded  Signal = "LLMResponded"
	ParseFailed   Signal = "ParseFailed"
	TaskCompleted Signal = "TaskCompleted"
)

// Lifecycle signals used by suspend/resume approval gates.
const (
	AwaitApproval Signal = "AwaitApproval"
	Approved      Signal = "Approved"
	Rejected      Signal = "Rejected"
)

// Validation signals used by the STL validate orchestrator.
const (
	ValidationPassed Signal = "ValidationPassed"
	ValidationFailed Signal = "ValidationFailed"
)

// Command is the single interface for all executable units of work. Undo is
// receipt-consuming: it receives the prior Result whose opaque Receipt the tool
// (originator) decodes to reverse its own effect; the engine and adapters never
// interpret the receipt (srd035-checkpoint-port R3).
type Command interface {
	Name() string
	Execute() Result
	Undo(prior Result) Result
}

// ContextCommand is an optional execution contract for commands that can block.
// SafeExecute cancels and joins these commands on timeout; legacy Command
// implementations retain detached timeout compatibility.
type ContextCommand interface {
	ExecuteContext(ctx context.Context) Result
}

// MonitorRecorderAware lets commands receive the tool-facing monitor recorder.
type MonitorRecorderAware interface {
	SetMonitorRecorder(monitor.ToolMetricsRecorder)
}

// CommandStateAware lets a command opt in to the read-only command-state view
// over prior steps' outputs. The engine injects the view before dispatch for
// commands that implement it, mirroring MonitorRecorderAware. The view exposes
// outputs only and never receipts (srd038-command-state-store R3).
type CommandStateAware interface {
	SetCommandState(CommandStateView)
}

// TraceContextAware lets a command receive the active dispatch span's context so
// it can propagate W3C trace context over a transport it owns (for example a
// REST client injecting traceparent). The engine injects it before dispatch,
// mirroring MonitorRecorderAware. Injection is uniform and carries no
// per-operation authoring surface (srd016-traceparent R4).
type TraceContextAware interface {
	SetTraceContext(oteltrace.SpanContext)
}

// NoopUndo returns a successful no-op undo result for commands that do not
// mutate rollback-managed state yet. Explicit Undo methods should call this so
// future work can grep for commands that still need real rollback behavior.
func NoopUndo(commandName string) Result {
	return Result{
		Signal:      ToolDone,
		CommandName: commandName,
		Output:      "undo: no-op",
	}
}

// Cost tracks resource consumption for a single Command dispatch.
type Cost struct {
	Duration  time.Duration `json:"duration"`
	TokensIn  int           `json:"tokens_in"`
	TokensOut int           `json:"tokens_out"`
	Dollars   float64       `json:"dollars"`
}

// ToolMetrics carries structured success/failure counts for a tool invocation.
type ToolMetrics struct {
	Total   int            `json:"total"`
	Passed  int            `json:"passed"`
	Failed  int            `json:"failed"`
	Details map[string]any `json:"details,omitempty"`
}

// Result carries the output of a Command execution.
type Result struct {
	Output      string
	Signal      Signal
	State       State
	Cost        Cost
	Err         error
	CommandName string
	Metrics     *ToolMetrics // nil when tool doesn't report metrics
	// Receipt is an opaque, tool-owned string encoded by the originating tool
	// and persisted verbatim on the execution Entry. The engine and every
	// checkpoint adapter treat it as opaque and never parse it
	// (srd035-checkpoint-port R3).
	Receipt string
}

// SpanOverride allows Commands to customize the Dispatch span name and
// creation-time attributes.
type SpanOverride interface {
	SpanName() string
	SpanCreationAttrs() []attribute.KeyValue
}

// Builder constructs a ready-to-execute Command from the previous Result.
type Builder interface {
	Build(res Result) Command
}

// Reverser is an opt-in Builder capability for reversible tools: BuildReverser
// constructs a fresh Command configured only for receipt-driven Undo, so a
// rollback can reverse a persisted step from its opaque Receipt alone, without
// the original invocation input. Builders that do not implement Reverser are
// treated as irreversible by the rollback receipt walk
// (srd035-checkpoint-port R3; #44 R2).
type Reverser interface {
	BuildReverser() Command
}

// CommandResolver looks up a Builder by command name.
type CommandResolver interface {
	Resolve(name string) (Builder, bool)
}
