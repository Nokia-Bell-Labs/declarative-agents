// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/attribute"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/telemetry/genai"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
)

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
	sm, ctx, cancel, err := prepareLoop(&params, ctx)
	if err != nil {
		return RunResult{}, err
	}
	if cancel != nil {
		defer cancel()
	}
	runTrace, runDone := startRunTrace(params)
	defer runDone()

	rr, err := coreLoop(sm, params, runTrace, ctx)
	recordRunResult(runTrace, rr)
	return rr, err
}

func prepareLoop(params *LoopParams, ctx context.Context) (*StateMachine, context.Context, context.CancelFunc, error) {
	if err := initFromMachine(params); err != nil {
		recordMachineInitError(params.Trace, err)
		return nil, nil, nil, fmt.Errorf("loop init: %w", err)
	}
	if err := validateLoopParams(params); err != nil {
		return nil, nil, nil, err
	}
	ctx, cancel := loopTimeoutContext(ctx, params.Budget)
	sm := NewStateMachine(params.Table, params.IsTerminal)
	params.Registry.Freeze()
	params.Trace.Event("init.registry_frozen",
		attribute.Int("tool_count", len(params.Registry.AllToolNames())),
	)
	return sm, ctx, cancel, nil
}

func recordMachineInitError(tr tracing.Tracer, err error) {
	if tr == nil {
		return
	}
	tr.Event("init.machine_failed", attribute.String("error", err.Error()))
}

func validateLoopParams(params *LoopParams) error {
	if params.Hooks.ValidateParams == nil {
		return nil
	}
	if err := params.Hooks.ValidateParams(params.Registry); err != nil {
		params.Trace.Event("init.validate_failed",
			attribute.String("error", err.Error()),
		)
		return err
	}
	return nil
}

func loopTimeoutContext(ctx context.Context, budget Budget) (context.Context, context.CancelFunc) {
	if budget.MaxDuration <= 0 {
		return ctx, nil
	}
	return context.WithTimeout(ctx, budget.MaxDuration)
}

func startRunTrace(params LoopParams) (tracing.Tracer, func()) {
	return params.Trace.Push(
		genai.AgentSpanName(params.AgentName),
		runSpanAttrs(params)...,
	)
}

func runSpanAttrs(params LoopParams) []attribute.KeyValue {
	return append(
		genai.AgentAttrs(params.AgentName, params.AgentVersion, params.ProviderName, params.ModelName),
		attribute.String("directory", params.Directory),
		attribute.Int("budget.max_iterations", params.Budget.MaxIterations),
		attribute.Int("budget.max_tokens", params.Budget.MaxTokens),
		attribute.Int64("budget.max_duration_ms", params.Budget.MaxDuration.Milliseconds()),
	)
}

func recordRunResult(tr tracing.Tracer, rr RunResult) {
	tr.SetAttributes(
		attribute.String("run.status", string(rr.Status)),
		attribute.String("run.final_state", string(rr.FinalState)),
		attribute.Int("run.iterations", rr.Iterations),
		genai.AttrUsageInputTokens.Int(rr.TokensIn),
		genai.AttrUsageOutputTokens.Int(rr.TokensOut),
		attribute.Int64("run.duration_ms", rr.Duration.Milliseconds()),
		genai.AttrResponseFinishReasons.String(mapRunStatusToFinishReason(rr.Status)),
	)
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
	case State("Succeeded"), State("Done"), State("Passed"), State("Completed"):
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
	case StatusSuspended:
		return "stop"
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
