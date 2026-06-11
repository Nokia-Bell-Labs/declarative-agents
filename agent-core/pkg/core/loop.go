// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/telemetry/genai"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/tracing"
)

// RunStatus describes the outcome of a completed run.
type RunStatus string

const (
	StatusSucceeded      RunStatus = "succeeded"
	StatusFailed         RunStatus = "failed"
	StatusBudgetExceeded RunStatus = "budget_exceeded"
	StatusCancelled      RunStatus = "cancelled"
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
	// ValidateParams checks that required builders exist in the registry.
	// Returns non-nil error to abort the run before it starts.
	ValidateParams func(reg *Registry) error

	// BudgetExceeded checks domain-specific budget dimensions beyond
	// the generic iteration/token/duration checks. Called with the
	// generic budget, current result, and iteration count.
	// Return true to trigger BudgetExhausted.
	BudgetExceeded func(budget Budget, rr RunResult, iterations int) bool

	// TerminalStatus maps a terminal State to a RunStatus string.
	// If nil, Succeeded maps to StatusSucceeded, BudgetExceeded state
	// maps to StatusBudgetExceeded, everything else to StatusFailed.
	TerminalStatus func(s State) RunStatus

	// OnResult is called after each command dispatch with the result.
	// The domain can update its own tracking (e.g., failure counters).
	// The returned RunResult replaces the current accumulator.
	OnResult func(rr RunResult, res Result) RunResult

	// AfterDispatch is called after each command dispatch with the
	// command and result. It may return a replacement signal (e.g. to
	// enforce domain-specific budget policies like consecutive parse
	// error limits). Return an empty signal to keep the original.
	AfterDispatch func(cmd Command, res Result) Signal

	// TaskCompletedSignal is the signal that indicates the task is done
	// and the summary should be captured. Defaults to "TaskCompleted".
	TaskCompletedSignal Signal
}

// LoopParams bundles all inputs for Loop.
//
// There are two initialization modes:
//
// Manual mode (existing): caller sets Registry, Table, IsTerminal.
//
// Declarative mode (new): caller sets MachineFile or MachineSpec (and
// optionally InitFunc to register tools). Loop loads or reuses the
// machine, validates actions against the registry, and builds the
// transition table.
//
// When MachineFile or MachineSpec is set, Table and IsTerminal are ignored and
// built internally. InitialState is derived from the machine spec.
type LoopParams struct {
	InitialState   State
	Prompt         string
	Registry       *Registry
	Table          TransitionTable
	IsTerminal     TerminalFunc
	Trace          tracing.Tracer
	Budget         Budget
	CommandTimeout time.Duration
	ModelName      string
	Directory      string
	Hooks          LoopHooks

	// Agent identity for OTel GenAI semantic conventions.
	AgentName    string // e.g. "generator", "planner"
	AgentVersion string // e.g. "v0.20260605.0"
	ProviderName string // e.g. "ollama"

	// Declarative initialization: Loop loads the machine from YAML,
	// validates that every action resolves to a registered tool, and
	// builds the transition table. Requires Registry to be populated
	// (either by the caller or via InitFunc).
	MachineFile string
	// MachineSpec optionally provides an already-loaded and validated machine
	// spec. When set, Loop uses it instead of reading MachineFile.
	MachineSpec *MachineSpec

	// InitFunc is called before the machine is loaded. Use it to
	// register tools with the registry (e.g. load from YAML, register
	// Go builders). Called with the registry that Loop will use.
	// When nil, the caller must populate the registry before calling Loop.
	InitFunc func(reg *Registry) error

	// ToolAction is the ActionFunc for "$tool" transitions (dynamic
	// dispatch). Required when the machine uses "$tool" actions.
	ToolAction ActionFunc
}

// Loop executes the generic agentic loop. It drives the state machine
// from InitialState to a terminal state, dispatching Commands through
// Dispatch, tracking budget, and collecting events.
//
// When MachineFile or MachineSpec is set, Loop owns initialization:
//  1. Create registry if nil
//  2. Call InitFunc to register tools
//  3. Load machine YAML, unless MachineSpec was provided
//  4. BuildTransitionTable (validates all actions resolve)
//  5. Freeze registry and enter the loop
func Loop(params LoopParams, ctx context.Context) (RunResult, error) {
	if err := initFromMachine(&params); err != nil {
		if params.Trace != nil {
			params.Trace.Event("init.machine_failed",
				attribute.String("error", err.Error()),
			)
		}
		return RunResult{}, fmt.Errorf("loop init: %w", err)
	}

	if params.Hooks.ValidateParams != nil {
		if err := params.Hooks.ValidateParams(params.Registry); err != nil {
			params.Trace.Event("init.validate_failed",
				attribute.String("error", err.Error()),
			)
			return RunResult{}, err
		}
	}

	if params.Budget.MaxDuration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, params.Budget.MaxDuration)
		defer cancel()
	}

	sm := NewStateMachine(params.Table, params.IsTerminal)
	params.Registry.Freeze()
	params.Trace.Event("init.registry_frozen",
		attribute.Int("tool_count", len(params.Registry.AllToolNames())),
	)

	agentSpanAttrs := append(
		genai.AgentAttrs(params.AgentName, params.AgentVersion, params.ProviderName, params.ModelName),
		attribute.String("directory", params.Directory),
		attribute.Int("budget.max_iterations", params.Budget.MaxIterations),
		attribute.Int("budget.max_tokens", params.Budget.MaxTokens),
		attribute.Int64("budget.max_duration_ms", params.Budget.MaxDuration.Milliseconds()),
	)

	runTrace, runDone := params.Trace.Push(
		genai.AgentSpanName(params.AgentName),
		agentSpanAttrs...,
	)
	defer runDone()

	rr, err := coreLoop(sm, params, runTrace, ctx)

	runTrace.SetAttributes(
		attribute.String("run.status", string(rr.Status)),
		attribute.String("run.final_state", string(rr.FinalState)),
		attribute.Int("run.iterations", rr.Iterations),
		genai.AttrUsageInputTokens.Int(rr.TokensIn),
		genai.AttrUsageOutputTokens.Int(rr.TokensOut),
		attribute.Int64("run.duration_ms", rr.Duration.Milliseconds()),
		genai.AttrResponseFinishReasons.String(mapRunStatusToFinishReason(rr.Status)),
	)

	return rr, err
}

func coreLoop(sm *StateMachine, p LoopParams, tr tracing.Tracer, ctx context.Context) (RunResult, error) {
	state := p.InitialState
	sig := Seed
	res := Result{Output: "Begin.", Signal: Seed}
	start := time.Now()

	taskCompletedSig := p.Hooks.TaskCompletedSignal
	if taskCompletedSig == "" {
		taskCompletedSig = "TaskCompleted"
	}

	var rr RunResult
	var iteration int

	for {
		if ctx.Err() != nil {
			tr.Event("run.cancelled",
				attribute.String("state", string(state)),
				attribute.String("reason", ctx.Err().Error()),
			)
			rr.Status = StatusCancelled
			rr.FinalState = state
			break
		}

		if coreBudgetExceeded(p.Budget, rr, iteration) || hookBudgetExceeded(p.Hooks, p.Budget, rr, iteration) {
			tr.Event("budget_exhausted",
				attribute.Int("iterations", iteration),
				attribute.Int("max_iterations", p.Budget.MaxIterations),
				attribute.Int("tokens_total", rr.TokensIn+rr.TokensOut),
				attribute.Int("max_tokens", p.Budget.MaxTokens),
			)
			sig = BudgetExhausted
		}

		nextState, cmd, err := sm.Step(state, sig, res)
		if err != nil {
			tr.Event("state.transition.unhandled",
				attribute.String("state", string(state)),
				attribute.String("signal", string(sig)),
				attribute.String("error", err.Error()),
			)
		} else {
			tr.Event("state.transition",
				attribute.String("from_state", string(state)),
				attribute.String("signal", string(sig)),
				attribute.String("to_state", string(nextState)),
			)
		}

		if sm.IsTerminal(nextState) {
			tr.Event("run.terminal",
				attribute.String("final_state", string(nextState)),
				attribute.String("status", string(resolveTerminalStatus(p.Hooks, nextState))),
			)
			rr.FinalState = nextState
			rr.Status = resolveTerminalStatus(p.Hooks, nextState)
			break
		}

		iteration++
		fromState := state
		state = nextState

		if cmd == nil {
			tr.Event("dispatch.nil_command",
				attribute.String("state", string(state)),
				attribute.String("signal", string(sig)),
			)
			rr.Status = StatusFailed
			rr.FinalState = state
			rr.LastError = fmt.Errorf("nil command in state %s (signal %s)", state, sig)
			break
		}

		res = Dispatch(cmd, tr, p.CommandTimeout)
		sig = res.Signal

		if p.Hooks.AfterDispatch != nil {
			if override := p.Hooks.AfterDispatch(cmd, res); override != "" {
				sig = override
			}
		}

		accumulateCost(&rr, res)
		if res.Err != nil {
			rr.LastError = res.Err
		}
		if res.Signal == taskCompletedSig {
			rr.Summary = res.Output
		}

		if p.Hooks.OnResult != nil {
			rr = p.Hooks.OnResult(rr, res)
		}

		rr.Events = append(rr.Events, RunEvent{
			Iteration:   iteration,
			Timestamp:   time.Now(),
			CommandName: res.CommandName,
			Signal:      res.Signal,
			Cost:        res.Cost,
			FromState:   fromState,
			ToState:     state,
		})

		emitIterationSpan(tr, iteration, res, fromState, state)
	}

	rr.Iterations = iteration
	rr.Duration = time.Since(start)

	log.Printf("run complete: status=%s iterations=%d tokens_in=%d tokens_out=%d duration=%s",
		rr.Status, rr.Iterations, rr.TokensIn, rr.TokensOut, rr.Duration)
	return rr, nil
}

func coreBudgetExceeded(b Budget, rr RunResult, iterations int) bool {
	if b.MaxIterations > 0 && iterations >= b.MaxIterations {
		return true
	}
	if b.MaxTokens > 0 && (rr.TokensIn+rr.TokensOut) >= b.MaxTokens {
		return true
	}
	return false
}

func hookBudgetExceeded(hooks LoopHooks, b Budget, rr RunResult, iterations int) bool {
	if hooks.BudgetExceeded != nil {
		return hooks.BudgetExceeded(b, rr, iterations)
	}
	return false
}

func resolveTerminalStatus(hooks LoopHooks, s State) RunStatus {
	if hooks.TerminalStatus != nil {
		return hooks.TerminalStatus(s)
	}
	return defaultTerminalStatus(s)
}

func defaultTerminalStatus(s State) RunStatus {
	switch s {
	case State("Succeeded"):
		return StatusSucceeded
	case State("BudgetExceeded"):
		return StatusBudgetExceeded
	default:
		return StatusFailed
	}
}

func mapRunStatusToFinishReason(s RunStatus) string {
	switch s {
	case StatusSucceeded:
		return "stop"
	case StatusBudgetExceeded:
		return "length"
	case StatusCancelled:
		return "cancelled"
	default:
		return "error"
	}
}

func accumulateCost(rr *RunResult, res Result) {
	rr.TokensIn += res.Cost.TokensIn
	rr.TokensOut += res.Cost.TokensOut
	rr.TotalCost += res.Cost.Dollars
}

func emitIterationSpan(tr tracing.Tracer, iter int, res Result, from, to State) {
	tr.SetAttributes(
		attribute.Int("iteration", iter),
		attribute.String("command", res.CommandName),
		attribute.String("signal", string(res.Signal)),
		attribute.String("from_state", string(from)),
		attribute.String("to_state", string(to)),
		attribute.Int("budget.tokens_in", res.Cost.TokensIn),
		attribute.Int("budget.tokens_out", res.Cost.TokensOut),
	)
}

// initFromMachine handles declarative initialization when MachineFile or
// MachineSpec is set. It creates the registry, calls InitFunc, loads or reuses
// the machine, validates actions, and populates Table/IsTerminal/InitialState
// on params.
func initFromMachine(params *LoopParams) error {
	if params.MachineFile == "" && params.MachineSpec == nil {
		return nil
	}

	if params.Registry == nil {
		params.Registry = NewRegistry()
	}

	if params.InitFunc != nil {
		if err := params.InitFunc(params.Registry); err != nil {
			return fmt.Errorf("init tools: %w", err)
		}
	}

	var spec MachineSpec
	if params.MachineSpec != nil {
		spec = *params.MachineSpec
	} else {
		loaded, err := LoadMachineSpec(params.MachineFile)
		if err != nil {
			return err
		}
		spec = loaded
	}

	table, isTerminal, err := BuildTransitionTable(spec, params.Registry, params.ToolAction)
	if err != nil {
		return err
	}

	params.Table = table
	params.IsTerminal = isTerminal
	params.InitialState = State(spec.InitialState)

	return nil
}

// ValidateBuilders is a convenience for creating ValidateParams hooks
// that check a list of required builder names.
func ValidateBuilders(names ...string) func(*Registry) error {
	return func(reg *Registry) error {
		for _, n := range names {
			if _, ok := reg.Resolve(n); !ok {
				return fmt.Errorf("initialization failed: missing builder %q", n)
			}
		}
		return nil
	}
}
