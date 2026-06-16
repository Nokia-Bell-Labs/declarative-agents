// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"context"
	"fmt"
	"log"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/observability/monitor"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/observability/tracing"
)

type loopRunner struct {
	sm               *StateMachine
	params           LoopParams
	trace            tracing.Tracer
	ctx              context.Context
	state            State
	signal           Signal
	result           Result
	run              RunResult
	iteration        int
	start            time.Time
	taskCompletedSig Signal
}

func coreLoop(sm *StateMachine, p LoopParams, tr tracing.Tracer, ctx context.Context) (RunResult, error) {
	r := newLoopRunner(sm, p, tr, ctx)
	r.recordStart()
	for !r.done() {
	}
	return r.finish(), nil
}

func newLoopRunner(sm *StateMachine, p LoopParams, tr tracing.Tracer, ctx context.Context) *loopRunner {
	sig, res := initialSignalResult(p)
	return &loopRunner{
		sm:               sm,
		params:           p,
		trace:            tr,
		ctx:              ctx,
		state:            p.InitialState,
		signal:           sig,
		result:           res,
		run:              p.InitialRun,
		iteration:        p.InitialRun.Iterations,
		start:            time.Now(),
		taskCompletedSig: taskCompletedSignal(p.Hooks),
	}
}

func initialSignalResult(p LoopParams) (Signal, Result) {
	if p.InitialSignal == "" {
		return Seed, Result{Output: "Begin.", Signal: Seed}
	}
	res := p.InitialResult
	if res.Signal == "" {
		res.Signal = p.InitialSignal
	}
	if res.Output == "" {
		res.Output = "Resume."
	}
	return p.InitialSignal, res
}

func taskCompletedSignal(hooks LoopHooks) Signal {
	if hooks.TaskCompletedSignal == "" {
		return "TaskCompleted"
	}
	return hooks.TaskCompletedSignal
}

func (r *loopRunner) recordStart() {
	recordMonitorRun(r.ctx, r.params.MonitorRecorder, monitor.RunSnapshot{
		RunID:     r.params.AgentName,
		Status:    "running",
		State:     string(r.state),
		Iteration: r.iteration,
	})
}

func (r *loopRunner) done() bool {
	if r.stopForContext() {
		return true
	}
	r.applyBudget()
	nextState, cmd, transitionSignal, metricLabels := r.nextTransition()
	if r.stopForTerminal(nextState) {
		return true
	}
	fromState := r.advance(nextState)
	if r.stopForNilCommand(cmd) {
		return true
	}
	r.dispatch(cmd, metricLabels, transitionSignal, fromState)
	return r.stopForSuspend()
}

func (r *loopRunner) stopForContext() bool {
	if r.ctx.Err() == nil {
		return false
	}
	r.trace.Event("run.cancelled",
		attribute.String("state", string(r.state)),
		attribute.String("reason", r.ctx.Err().Error()),
	)
	r.run.Status = StatusCancelled
	r.run.FinalState = r.state
	return true
}

func (r *loopRunner) applyBudget() {
	if !coreBudgetExceeded(r.params.Budget, r.run, r.iteration) &&
		!hookBudgetExceeded(r.params.Hooks, r.params.Budget, r.run, r.iteration) {
		return
	}
	r.trace.Event("budget_exhausted",
		attribute.Int("iterations", r.iteration),
		attribute.Int("max_iterations", r.params.Budget.MaxIterations),
		attribute.Int("tokens_total", r.run.TokensIn+r.run.TokensOut),
		attribute.Int("max_tokens", r.params.Budget.MaxTokens),
	)
	r.signal = BudgetExhausted
}

func (r *loopRunner) nextTransition() (State, Command, Signal, MetricLabels) {
	transitionSignal := r.signal
	labels := transitionMetricLabels(r.params.MachineSpec, r.state, transitionSignal)
	nextState, cmd, err := r.sm.Step(r.state, transitionSignal, r.result)
	if err != nil {
		r.recordUnhandledTransition(err)
	} else {
		r.recordTransition(nextState)
	}
	return nextState, cmd, transitionSignal, labels
}

func (r *loopRunner) recordUnhandledTransition(err error) {
	r.trace.Event("state.transition.unhandled",
		attribute.String("state", string(r.state)),
		attribute.String("signal", string(r.signal)),
		attribute.String("error", err.Error()),
	)
}

func (r *loopRunner) recordTransition(nextState State) {
	r.trace.Event("state.transition",
		attribute.String("from_state", string(r.state)),
		attribute.String("signal", string(r.signal)),
		attribute.String("to_state", string(nextState)),
	)
}

func (r *loopRunner) stopForTerminal(nextState State) bool {
	if !r.sm.IsTerminal(nextState) {
		return false
	}
	status := resolveTerminalStatus(r.params.Hooks, nextState)
	r.trace.Event("run.terminal",
		attribute.String("final_state", string(nextState)),
		attribute.String("status", string(status)),
	)
	r.run.FinalState = nextState
	r.run.Status = status
	return true
}

func (r *loopRunner) advance(nextState State) State {
	fromState := r.state
	r.iteration++
	r.state = nextState
	return fromState
}

func (r *loopRunner) stopForNilCommand(cmd Command) bool {
	if cmd != nil {
		return false
	}
	r.trace.Event("dispatch.nil_command",
		attribute.String("state", string(r.state)),
		attribute.String("signal", string(r.signal)),
	)
	r.run.Status = StatusFailed
	r.run.FinalState = r.state
	r.run.LastError = fmt.Errorf("nil command in state %s (signal %s)", r.state, r.signal)
	return true
}

func (r *loopRunner) dispatch(cmd Command, labels MetricLabels, transitionSignal Signal, fromState State) {
	r.result = dispatchWithMonitor(cmd, r.trace, r.params.CommandTimeout, r.params.MonitorRecorder, r.dispatchContext(labels))
	r.signal = r.result.Signal
	r.applyAfterDispatch(cmd)
	r.accumulateResult()
	r.recordResultEvent(fromState)
	r.recordHistory(cmd, fromState, transitionSignal)
	emitIterationSpan(r.trace, r.iteration, r.result, fromState, r.state)
}

func (r *loopRunner) dispatchContext(labels MetricLabels) monitor.DispatchContext {
	return monitor.DispatchContext{
		RunID:        r.params.AgentName,
		AgentName:    r.params.AgentName,
		State:        string(r.state),
		Iteration:    r.iteration,
		MetricLabels: labels,
	}
}

func (r *loopRunner) applyAfterDispatch(cmd Command) {
	if r.params.Hooks.AfterDispatch == nil {
		return
	}
	if override := r.params.Hooks.AfterDispatch(cmd, r.result); override != "" {
		r.signal = override
	}
}

func (r *loopRunner) accumulateResult() {
	accumulateCost(&r.run, r.result)
	if r.result.Err != nil {
		r.run.LastError = r.result.Err
	}
	if r.result.Signal == r.taskCompletedSig {
		r.run.Summary = r.result.Output
	}
	if r.params.Hooks.OnResult != nil {
		r.run = r.params.Hooks.OnResult(r.run, r.result)
	}
}

func (r *loopRunner) recordResultEvent(fromState State) {
	event := RunEvent{
		Iteration:   r.iteration,
		Timestamp:   time.Now(),
		CommandName: r.result.CommandName,
		Signal:      r.result.Signal,
		Cost:        r.result.Cost,
		FromState:   fromState,
		ToState:     r.state,
	}
	r.run.Events = append(r.run.Events, event)
	recordMonitorEvent(r.ctx, r.params.MonitorRecorder, event)
	recordMonitorRun(r.ctx, r.params.MonitorRecorder, r.runSnapshot())
}

func (r *loopRunner) runSnapshot() monitor.RunSnapshot {
	return monitor.RunSnapshot{
		RunID:     r.params.AgentName,
		Status:    "running",
		State:     string(r.state),
		Signal:    string(r.signal),
		Iteration: r.iteration,
	}
}

func (r *loopRunner) recordHistory(cmd Command, fromState State, transitionSignal Signal) {
	if !historyEnabled(r.params) {
		return
	}
	ref := captureWorkspaceRef(r.params, r.trace, r.ctx, r.iteration, r.result.CommandName)
	entry := newHistoryEntry(r.iteration, cmd, r.result, fromState, r.state, transitionSignal, ref, r.trace)
	r.run.History = append(r.run.History, entry)
}

func (r *loopRunner) stopForSuspend() bool {
	if r.signal != AwaitApproval {
		return false
	}
	if err := persistSuspendCheckpoint(r.ctx, r.params, r.trace, &r.run, r.state, r.signal, r.iteration); err != nil {
		r.run.Status = StatusFailed
		r.run.FinalState = r.state
		r.run.LastError = err
		return true
	}
	r.trace.Event("run.suspended",
		attribute.String("state", string(r.state)),
		attribute.Int("iteration", r.iteration),
	)
	r.run.Status = StatusSuspended
	r.run.FinalState = r.state
	return true
}

func (r *loopRunner) finish() RunResult {
	r.run.Iterations = r.iteration
	r.run.Duration = time.Since(r.start)
	recordMonitorRun(r.ctx, r.params.MonitorRecorder, monitor.RunSnapshot{
		RunID:     r.params.AgentName,
		Status:    string(r.run.Status),
		State:     string(r.run.FinalState),
		Signal:    string(r.signal),
		Iteration: r.iteration,
	})
	log.Printf("run complete: status=%s iterations=%d tokens_in=%d tokens_out=%d duration=%s",
		r.run.Status, r.run.Iterations, r.run.TokensIn, r.run.TokensOut, r.run.Duration)
	return r.run
}
