// Copyright (c) 2026 Nokia. All rights reserved.

package bench

import (
	"encoding/json"
	"fmt"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// Bench-specific signals routed by the state machine.
const (
	ExperimentRequested core.Signal = "ExperimentRequested"
	Shutdown            core.Signal = "Shutdown"
	EvalCompleted       core.Signal = "EvalCompleted"
	EvalFailed          core.Signal = "EvalFailed"
)

// UserAction represents an action submitted by the web UI via
// POST /api/v1/actions. The state machine routes it by mapping
// Type to a core.Signal.
type UserAction struct {
	Type   string                 `json:"type"`
	Config map[string]interface{} `json:"config,omitempty"`
}

// Signal converts the user action type to a core.Signal.
func (a UserAction) Signal() core.Signal {
	switch a.Type {
	case "launch_eval":
		return ExperimentRequested
	case "shutdown":
		return Shutdown
	default:
		return core.CommandError
	}
}

// String returns a JSON representation of the action for Result.Output.
func (a UserAction) String() string {
	b, _ := json.Marshal(a)
	return string(b)
}

// BenchState holds shared mutable state for bench tools, analogous
// to EvalState for eval tools. The serve_ui tool creates and manages
// the HTTP server; launch_eval reads experiment config from the last
// user action.
type BenchState struct {
	Srv        *Server
	Addr       string
	ActionCh   chan UserAction
	LastAction UserAction
	started    bool
}

// NewBenchState creates a BenchState with the given server config.
func NewBenchState(cfg ServerConfig) *BenchState {
	ch := make(chan UserAction, 1)
	srv := NewServer(cfg, ch)
	return &BenchState{
		Srv:      srv,
		Addr:     cfg.Addr,
		ActionCh: ch,
	}
}

// EnsureRunning starts the HTTP server in a background goroutine
// if it hasn't been started yet.
func (bs *BenchState) EnsureRunning() {
	if bs.started {
		return
	}
	bs.started = true
	go func() {
		if err := bs.Srv.ListenAndServe(bs.Addr); err != nil {
			fmt.Printf("bench server error: %v\n", err)
		}
	}()
}
