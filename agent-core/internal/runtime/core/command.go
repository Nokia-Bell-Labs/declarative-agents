// Copyright (c) 2026 Nokia. All rights reserved.

// Package core provides the generic agentic loop engine: a state machine,
// command dispatch, tool registry, budget tracking, and tracing.
package core

import (
	"time"

	"go.opentelemetry.io/otel/attribute"

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

// Command is the single interface for all executable units of work.
type Command interface {
	Name() string
	Execute() Result
	Undo() Result
}

// MonitorRecorderAware lets commands receive the tool-facing monitor recorder.
type MonitorRecorderAware interface {
	SetMonitorRecorder(monitor.ToolMetricsRecorder)
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
	Cost        Cost
	Err         error
	CommandName string
	Metrics     *ToolMetrics // nil when tool doesn't report metrics
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

// CommandResolver looks up a Builder by command name.
type CommandResolver interface {
	Resolve(name string) (Builder, bool)
}
