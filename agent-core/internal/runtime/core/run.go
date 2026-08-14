// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"encoding/json"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/monitor"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
)

// AttrRequestID is the repository-owned request identity attribute. OpenTelemetry
// defines no general semantic convention for an application request identifier.
const AttrRequestID attribute.Key = "declarative_agents.request.id"

// RunStatus describes the outcome of a completed run.
type RunStatus string

const (
	StatusSucceeded      RunStatus = "succeeded"
	StatusFailed         RunStatus = "failed"
	StatusBudgetExceeded RunStatus = "budget_exceeded"
	StatusCancelled      RunStatus = "cancelled"
	StatusSuspended      RunStatus = "suspended"
)

// Budget controls iteration, token, and wall-clock limits for a run.
// Domain agents extend budget checking via LoopHooks.BudgetExceeded.
type Budget struct {
	MaxIterations int
	MaxTokens     int
	MaxDuration   time.Duration
}

// RunEvent records one command dispatch.
type RunEvent struct {
	Iteration   int       `json:"iteration"`
	Timestamp   time.Time `json:"timestamp"`
	CommandName string    `json:"command_name"`
	Signal      Signal    `json:"signal"`
	Cost        Cost      `json:"cost"`
	FromState   State     `json:"from_state"`
	ToState     State     `json:"to_state"`
}

// RunResult carries the outcome of a complete run.
type RunResult struct {
	Status     RunStatus     `json:"status"`
	Iterations int           `json:"iterations"`
	TokensIn   int           `json:"tokens_in"`
	TokensOut  int           `json:"tokens_out"`
	Duration   time.Duration `json:"-"`
	TotalCost  float64       `json:"total_cost"`
	FinalState State         `json:"final_state"`
	LastError  error         `json:"-"`
	Summary    string        `json:"summary"`
	Events     []RunEvent    `json:"events"`
}

// MarshalJSON implements custom JSON serialization for RunResult.
func (rr RunResult) MarshalJSON() ([]byte, error) {
	type Alias RunResult
	var lastErr *string
	if rr.LastError != nil {
		s := rr.LastError.Error()
		lastErr = &s
	}
	return json.Marshal(&struct {
		Alias
		Duration  string  `json:"duration"`
		LastError *string `json:"last_error"`
	}{
		Alias:     Alias(rr),
		Duration:  rr.Duration.String(),
		LastError: lastErr,
	})
}

// LoopHooks provides domain-specific callbacks for the generic loop.
// All callbacks are optional; nil means use default behavior.
type LoopHooks struct {
	BudgetExceeded       func(budget Budget, rr RunResult, iterations int) bool
	TerminalStatus       func(s State) RunStatus
	OnResult             func(rr RunResult, res Result) RunResult
	TaskCompletedSignal  Signal
	SnapshotConversation func() (json.RawMessage, error)
	SnapshotDomain       func() (json.RawMessage, error)
	// RestoreSnapshot rehydrates domain-owned state after request-signal resume
	// has loaded and validated a checkpoint, before the resumed loop dispatches.
	// Ordinary run/resume callers keep their existing explicit restore path.
	RestoreSnapshot func(AgentSnapshot) error
}

// LoopParams bundles all inputs for Loop.
type LoopParams struct {
	InitialState  State
	InitialSignal Signal
	InitialResult Result
	// PreserveInitialResultOutput keeps an explicitly empty initial output
	// empty. Signal admission uses it after fail-closed payload redaction;
	// existing model and resume callers retain the historical "Resume." default.
	PreserveInitialResultOutput bool
	InitialRun                  RunResult
	// InitialExecution seeds the loop's Execution log so a resumed run continues
	// appending to the persisted history instead of starting a fresh log (srd035).
	InitialExecution Execution
	// InitialIterator restores an in-progress sequential for_each frame. It is
	// populated by LoadResume from the typed checkpoint snapshot.
	InitialIterator *IteratorSnapshot
	Registry        *Registry
	Table           TransitionTable
	IsTerminal      TerminalFunc
	Trace           tracing.Tracer
	Budget          Budget
	CommandTimeout  time.Duration
	ModelName       string
	Directory       string
	Hooks           LoopHooks
	// RunID identifies one logical run across checkpoint, monitor, and trace
	// records. It remains stable when that run is resumed.
	RunID string
	// RequestID identifies the request that owns a request-scoped run. It is
	// empty for ordinary run-scoped agents.
	RequestID string
	// ConversationID groups all spans belonging to one conversation. When empty,
	// the loop uses RunID so ordinary and resumed runs retain stable identity.
	ConversationID string
	AgentName      string
	AgentVersion   string
	ProviderName   string
	MachineFile    string
	MachineSpec    *MachineSpec
	// Program identifies the immutable declarative profile whose tools the run
	// registered. It is persisted for cross-process receipt rollback.
	Program    ProgramRef
	InitFunc   func(reg *Registry) error
	ToolAction ActionFunc
	// Checkpoint is the typed persistence port (srd035). The loop saves the
	// current Position and Execution through it after each dispatch cycle. A nil
	// value defaults to NoopCheckpoint, preserving disabled-mode behavior.
	Checkpoint      Checkpoint
	MonitorRecorder monitor.RuntimeRecorder
	// CommandStateObserver receives the execution log after each dispatch so a
	// live command-state source stays current for a background monitor server. A
	// nil value keeps disabled-mode behavior (srd033-monitor-rest-api R7.1).
	CommandStateObserver CommandStateObserver
}
