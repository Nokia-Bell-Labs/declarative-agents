// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"encoding/json"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/monitor"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
)

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
// Domain agents extend budget checking via LoopHooks.BudgetExceeded
// and LoopHooks.AfterDispatch for domain-specific policies.
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
	History    History       `json:"history,omitempty"`
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
	ValidateParams       func(reg *Registry) error
	BudgetExceeded       func(budget Budget, rr RunResult, iterations int) bool
	TerminalStatus       func(s State) RunStatus
	OnResult             func(rr RunResult, res Result) RunResult
	AfterDispatch        func(cmd Command, res Result) Signal
	TaskCompletedSignal  Signal
	SnapshotConversation func() (json.RawMessage, error)
	SnapshotDomain       func() (json.RawMessage, error)
}

// LoopParams bundles all inputs for Loop.
type LoopParams struct {
	InitialState  State
	InitialSignal Signal
	InitialResult Result
	InitialRun    RunResult
	// InitialExecution seeds the loop's Execution log so a resumed run continues
	// appending to the persisted history instead of starting a fresh log (srd035).
	InitialExecution Execution
	Prompt           string
	Registry         *Registry
	Table            TransitionTable
	IsTerminal       TerminalFunc
	Trace            tracing.Tracer
	Budget           Budget
	CommandTimeout   time.Duration
	ModelName        string
	Directory        string
	Hooks            LoopHooks
	AgentName        string
	AgentVersion     string
	ProviderName     string
	MachineFile      string
	MachineSpec      *MachineSpec
	InitFunc         func(reg *Registry) error
	ToolAction       ActionFunc
	StateStore       StateStore
	Workspace        Workspace
	CheckpointPolicy CheckpointPolicy
	// Checkpoint is the typed persistence port (srd035). The loop saves the
	// current Position and Execution through it after each dispatch cycle. A nil
	// value defaults to NoopCheckpoint, preserving disabled-mode behavior.
	Checkpoint      Checkpoint
	MonitorRecorder monitor.RuntimeRecorder
}
