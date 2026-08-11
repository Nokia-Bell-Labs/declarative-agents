// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"context"
	"fmt"
	"log"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/monitor"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
)

type loopRunner struct {
	sm                *StateMachine
	params            LoopParams
	trace             tracing.Tracer
	ctx               context.Context
	state             State
	signal            Signal
	result            Result
	run               RunResult
	iteration         int
	start             time.Time
	taskCompletedSig  Signal
	checkpoint        Checkpoint
	checkpointEnabled bool
	execution         Execution
	iterator          *IteratorSnapshot
	// checkpointSaveErr holds a classified failure to construct or save the
	// current stateful checkpoint. The loop stops at that dispatch boundary so
	// it never claims that an unpersisted Position/Execution pair is resumable.
	checkpointSaveErr error
}

func coreLoop(sm *StateMachine, p LoopParams, tr tracing.Tracer, ctx context.Context) (RunResult, error) {
	r := newLoopRunner(sm, p, tr, ctx)
	r.recordStart()
	for !r.done() {
	}
	result := r.finish()
	if r.checkpointSaveErr != nil {
		return result, result.LastError
	}
	return result, nil
}

func newLoopRunner(sm *StateMachine, p LoopParams, tr tracing.Tracer, ctx context.Context) *loopRunner {
	sig, res := initialSignalResult(p)
	return &loopRunner{
		sm:                sm,
		params:            p,
		trace:             tr,
		ctx:               ctx,
		state:             p.InitialState,
		signal:            sig,
		result:            res,
		run:               p.InitialRun,
		iteration:         p.InitialRun.Iterations,
		start:             time.Now(),
		taskCompletedSig:  taskCompletedSignal(p.Hooks),
		checkpoint:        resolveCheckpoint(p.Checkpoint),
		checkpointEnabled: checkpointPersistenceEnabled(p.Checkpoint),
		execution:         cloneExecution(p.InitialExecution),
		iterator:          cloneIteratorSnapshot(p.InitialIterator),
	}
}

func checkpointPersistenceEnabled(checkpoint Checkpoint) bool {
	switch checkpoint.(type) {
	case nil, NoopCheckpoint, *NoopCheckpoint:
		return false
	default:
		return true
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
		RunID:     r.params.RunID,
		Status:    "running",
		State:     string(r.state),
		Iteration: r.iteration,
	})
	// A resumed run seeds the observer with its persisted history so the monitor
	// resolves prior-step labels before the first fresh dispatch.
	r.observeCommandState()
}

func (r *loopRunner) done() bool {
	if r.stopForContext() {
		return true
	}
	r.applyBudget()
	if r.iterator != nil && r.signal != BudgetExhausted {
		return r.doneIterator()
	}
	nextState, cmd, transitionSignal, commandStateLabel, metricLabels, err := r.nextTransition()
	if err != nil {
		return r.stopForUnhandledTransition(err)
	}
	if r.iterator != nil && effectiveForEachMode(r.iterator.Spec) == ForEachParallel {
		r.state = nextState
		return r.dispatchParallelIterator()
	}
	if cmd == nil {
		if r.stopForTerminal(nextState) {
			return true
		}
		r.advance(nextState)
		return r.stopForNilCommand(cmd)
	}
	fromState := r.advance(nextState)
	r.dispatch(cmd, metricLabels, transitionSignal, commandStateLabel, fromState)
	return r.stopAfterDispatch(nextState)
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

func (r *loopRunner) nextTransition() (State, Command, Signal, string, MetricLabels, error) {
	transitionSignal := r.signal
	commandStateLabel := transitionCommandStateLabel(r.params.MachineSpec, r.state, transitionSignal)
	labels := transitionMetricLabels(r.params.MachineSpec, r.state, transitionSignal)
	nextState, cmd, err := r.sm.Step(r.state, transitionSignal, r.result)
	if err != nil {
		return nextState, cmd, transitionSignal, commandStateLabel, labels, err
	}
	nextState, cmd, commandStateLabel = r.prepareIterator(
		transitionSignal, nextState, cmd, commandStateLabel,
	)
	r.recordTransition(nextState)
	return nextState, cmd, transitionSignal, commandStateLabel, labels, nil
}

func transitionCommandStateLabel(spec *MachineSpec, state State, signal Signal) string {
	if spec == nil {
		return ""
	}
	for _, transition := range spec.Transitions {
		if transition.State == string(state) && transition.Signal == string(signal) {
			return transition.Label
		}
	}
	return ""
}

func (r *loopRunner) stopForUnhandledTransition(err error) bool {
	r.trace.Event("state.transition.unhandled",
		attribute.String("state", string(r.state)),
		attribute.String("signal", string(r.signal)),
		attribute.String("error", err.Error()),
	)
	r.run.Status = StatusFailed
	r.run.FinalState = r.state
	r.run.LastError = err
	return true
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
	// An actionless terminal transition has no dispatch cycle to persist it.
	// Move to the actual terminal state without incrementing the dispatch count,
	// then save the same Execution with its terminal Position. A transition whose
	// action targeted a terminal state already saved this Position in dispatch,
	// so state equality prevents a duplicate Save and duplicate step commit.
	if r.state != nextState {
		r.state = nextState
		r.saveTerminalCheckpoint()
	}
	status := resolveTerminalStatus(r.params.Hooks, r.params.MachineSpec, nextState)
	if r.checkpointSaveErr != nil {
		status = StatusFailed
		r.run.LastError = fmt.Errorf(
			"terminal checkpoint not persisted at iteration %d: %w",
			r.iteration,
			r.checkpointSaveErr,
		)
		r.trace.Event("run.terminal_persist_failed",
			attribute.String("state", string(nextState)),
			attribute.Int("iteration", r.iteration),
			attribute.String("error", r.checkpointSaveErr.Error()),
		)
	}
	r.trace.Event("run.terminal",
		attribute.String("final_state", string(nextState)),
		attribute.String("status", string(status)),
	)
	r.run.FinalState = nextState
	r.run.Status = status
	return true
}

// saveTerminalCheckpoint persists an actionless terminal transition without
// appending an Entry: no command was dispatched, so Execution must remain
// unchanged. Stateful adapters may record a terminal-finalization commit before
// applying their terminal branch lifecycle.
func (r *loopRunner) saveTerminalCheckpoint() {
	if !r.checkpointEnabled {
		return
	}
	pos := dispatchPosition(r.state, r.signal, r.iteration, &r.run)
	pos.Snapshot.Iterator = cloneIteratorSnapshot(r.iterator)
	pos.Snapshot.Program = r.params.Program
	if err := r.foldConversation(&pos); err != nil {
		r.recordCheckpointFailure(err)
		return
	}
	if err := r.checkpoint.Save(pos, r.execution); err != nil {
		r.recordCheckpointFailure(fmt.Errorf(
			"%w: adapter %T Save at iteration %d: %w",
			ErrCheckpointSaveFailed,
			r.checkpoint,
			r.iteration,
			err,
		))
	}
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

func (r *loopRunner) dispatch(
	cmd Command,
	labels MetricLabels,
	transitionSignal Signal,
	commandStateLabel string,
	fromState State,
) {
	injectCommandStateBindings(cmd, r.execution, r.iteratorBindings())
	r.result = dispatchWithMonitorContext(
		r.ctx, cmd, r.trace, r.params.CommandTimeout, r.params.MonitorRecorder, r.dispatchContext(labels),
	)
	r.signal = r.result.Signal
	r.recordIteratorOutcome()
	r.accumulateResult()
	r.recordResultEvent(fromState)
	r.saveCheckpoint(fromState, transitionSignal, commandStateLabel)
	emitIterationSpan(r.trace, r.iteration, r.result, fromState, r.state)
}

// saveCheckpoint persists the updated Position and appended Execution through the
// checkpoint port after each dispatch cycle (srd035-checkpoint-port R6.1).
// Stateful checkpoint construction and Save failures stop the run at this
// boundary. NoopCheckpoint skips snapshot and Save work entirely.
func (r *loopRunner) saveCheckpoint(fromState State, transitionSignal Signal, commandStateLabel string) {
	r.execution = append(
		r.execution,
		dispatchEntry(r.iteration, fromState, r.state, transitionSignal, commandStateLabel, r.result),
	)
	r.observeCommandState()
	if !r.checkpointEnabled {
		return
	}
	pos := dispatchPosition(r.state, r.signal, r.iteration, &r.run)
	pos.Snapshot.Iterator = cloneIteratorSnapshot(r.iterator)
	pos.Snapshot.Program = r.params.Program
	if err := r.foldConversation(&pos); err != nil {
		r.recordCheckpointFailure(err)
		return
	}
	if err := r.checkpoint.Save(pos, r.execution); err != nil {
		r.recordCheckpointFailure(fmt.Errorf(
			"%w: adapter %T Save at iteration %d: %w",
			ErrCheckpointSaveFailed,
			r.checkpoint,
			r.iteration,
			err,
		))
	}
}

// foldConversation folds the domain-owned conversation into the resumable
// Position so the typed checkpoint port persists it alongside loop state. Core
// cannot import the llm package, so the conversation arrives through the
// SnapshotConversation hook. Failure leaves the prior Position/Execution unit
// untouched rather than saving a Position with its required conversation
// silently omitted (srd035-checkpoint-port R1.2, R4, R6.1).
func (r *loopRunner) foldConversation(pos *Position) error {
	if r.params.Hooks.SnapshotConversation == nil {
		return nil
	}
	conversation, err := r.params.Hooks.SnapshotConversation()
	if err != nil {
		r.trace.Event("checkpoint.conversation_snapshot_failed",
			attribute.Int("iteration", r.iteration),
			attribute.String("error", err.Error()),
		)
		return fmt.Errorf("%w at iteration %d: %w", ErrConversationSnapshotFailed, r.iteration, err)
	}
	pos.Snapshot.Conversation = conversation
	return nil
}

func (r *loopRunner) recordCheckpointFailure(err error) {
	r.checkpointSaveErr = err
	r.trace.Event("checkpoint.save_failed",
		attribute.Int("iteration", r.iteration),
		attribute.String("error", err.Error()),
	)
}

func (r *loopRunner) dispatchContext(labels MetricLabels) monitor.DispatchContext {
	return monitor.DispatchContext{
		RunID:        r.params.RunID,
		AgentName:    r.params.AgentName,
		State:        string(r.state),
		Iteration:    r.iteration,
		MetricLabels: labels,
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
		RunID:     r.params.RunID,
		Status:    "running",
		State:     string(r.state),
		Signal:    string(r.signal),
		Iteration: r.iteration,
	}
}

func (r *loopRunner) stopForSuspend() bool {
	if r.signal != AwaitApproval {
		return false
	}
	// Suspend persistence is mandatory: dispatch's saveCheckpoint just ran, so
	// if that Save failed there is no resumable checkpoint and the run must not
	// report StatusSuspended (srd025 R5.3). Treat it as a terminal lifecycle
	// error instead of a resumable suspend.
	if r.checkpointSaveErr != nil {
		r.trace.Event("run.suspend_persist_failed",
			attribute.String("state", string(r.state)),
			attribute.Int("iteration", r.iteration),
			attribute.String("error", r.checkpointSaveErr.Error()),
		)
		r.run.Status = StatusFailed
		r.run.FinalState = r.state
		r.run.LastError = fmt.Errorf("suspend checkpoint not persisted at iteration %d: %w", r.iteration, r.checkpointSaveErr)
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

func (r *loopRunner) stopAfterDispatch(nextState State) bool {
	if r.stopForSuspend() {
		return true
	}
	if r.stopForCheckpointFailure() {
		return true
	}
	if r.stopForContext() {
		return true
	}
	return r.stopForTerminal(nextState)
}

func (r *loopRunner) stopForCheckpointFailure() bool {
	if r.checkpointSaveErr == nil {
		return false
	}
	r.trace.Event("run.checkpoint_persist_failed",
		attribute.String("state", string(r.state)),
		attribute.Int("iteration", r.iteration),
		attribute.String("error", r.checkpointSaveErr.Error()),
	)
	r.run.Status = StatusFailed
	r.run.FinalState = r.state
	r.run.LastError = fmt.Errorf("run checkpoint not persisted: %w", r.checkpointSaveErr)
	return true
}

func (r *loopRunner) finish() RunResult {
	r.run.Iterations = r.iteration
	r.run.Duration = time.Since(r.start)
	recordMonitorRun(r.ctx, r.params.MonitorRecorder, monitor.RunSnapshot{
		RunID:     r.params.RunID,
		Status:    string(r.run.Status),
		State:     string(r.run.FinalState),
		Signal:    string(r.signal),
		Iteration: r.iteration,
	})
	log.Printf("run complete: status=%s iterations=%d tokens_in=%d tokens_out=%d duration=%s",
		r.run.Status, r.run.Iterations, r.run.TokensIn, r.run.TokensOut, r.run.Duration)
	return r.run
}
