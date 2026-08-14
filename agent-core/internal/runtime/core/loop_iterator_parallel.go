// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"go.opentelemetry.io/otel/attribute"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/monitor"
)

type parallelIteratorCommand struct {
	index   int
	command Command
	context monitor.DispatchContext
}

type parallelIteratorResult struct {
	index  int
	result Result
}

func (r *loopRunner) dispatchParallelIterator() bool {
	frame := r.iterator
	start := frame.NextIndex
	commands, err := r.parallelIteratorCommands(start)
	if err != nil {
		r.result = Result{Signal: CommandError, CommandName: "for_each", Err: err, Output: err.Error()}
		frame.Outcomes = append(frame.Outcomes, IteratorOutcome{
			Index: start, Input: cloneRawMessage(frame.Items[start]),
			CommandName: r.result.CommandName, Result: DigestResult(r.result),
		})
		frame.NextIndex = start + 1
		frame.Halted = true
		return false
	}
	if len(commands) == 0 {
		return r.stopForContext()
	}

	results := r.runParallelIteratorCommands(commands)
	suspend := r.commitParallelIteratorResults(results, start)
	if suspend != nil {
		r.result = *suspend
		r.signal = AwaitApproval
		return r.stopForSuspend()
	}
	if r.stopForCheckpointFailure() || r.stopForContext() {
		return true
	}
	return false
}

func (r *loopRunner) commitParallelIteratorResults(
	results []parallelIteratorResult,
	start int,
) *Result {
	frame := r.iterator
	var suspend *Result
	for _, completed := range results {
		r.iteration++
		r.result = completed.result
		r.signal = completed.result.Signal
		r.recordIteratorOutcome()
		r.accumulateResult()
		fromState := frame.BodyState
		if completed.index == 0 && start == 0 {
			fromState = frame.TransitionState
		}
		r.recordResultEvent(fromState)
		r.saveCheckpoint(fromState, frame.TransitionSignal, frame.Label)
		emitIterationSpan(r.trace, r.iteration, r.result, fromState, r.state)
		if r.signal == AwaitApproval {
			captured := r.result
			suspend = &captured
		}
	}
	return suspend
}

func (r *loopRunner) parallelIteratorCommands(start int) ([]parallelIteratorCommand, error) {
	frame := r.iterator
	end := len(frame.Items)
	if limit := r.params.Budget.MaxIterations; limit > 0 {
		available := limit - r.iteration
		if available < end-start {
			end = start + max(available, 0)
		}
	}
	builder, ok := r.params.Registry.Resolve(frame.Action)
	if !ok {
		return nil, fmt.Errorf("iterator action %q is unavailable", frame.Action)
	}
	labels := transitionMetricLabels(r.params.MachineSpec, frame.TransitionState, frame.TransitionSignal)
	commands := make([]parallelIteratorCommand, 0, end-start)
	for index := start; index < end; index++ {
		input := r.result
		input.State = frame.BodyState
		command := builder.Build(input)
		if err := validateParallelIteratorCommand(frame.Action, index, command); err != nil {
			return nil, err
		}
		injectCommandStateBindings(command, r.execution, map[string]string{
			frame.Spec.As: string(frame.Items[index]),
		})
		dispatchCtx := r.dispatchContext(labels)
		dispatchCtx.Iteration = r.iteration + index - start + 1
		r.trace.Event("for_each.item",
			attribute.String("as", frame.Spec.As),
			attribute.Int("index", index),
			attribute.Int("items", len(frame.Items)),
		)
		commands = append(commands, parallelIteratorCommand{
			index: index, command: command, context: dispatchCtx,
		})
	}
	return commands, nil
}

func validateParallelIteratorCommand(action string, index int, command Command) error {
	if command == nil {
		return fmt.Errorf("iterator action %q built a nil command for item %d", action, index)
	}
	if _, serial := command.(SerialDispatchOnly); serial {
		return fmt.Errorf(
			"iterator action %q requires serial dispatch and cannot run in parallel mode",
			action,
		)
	}
	return nil
}

func (r *loopRunner) runParallelIteratorCommands(commands []parallelIteratorCommand) []parallelIteratorResult {
	ctx, cancel := context.WithCancel(r.ctx)
	defer cancel()
	jobs := make(chan parallelIteratorCommand)
	results := make(chan parallelIteratorResult, len(commands))
	workers := min(r.iterator.Spec.MaxConcurrency, len(commands))
	var wg sync.WaitGroup
	wg.Add(workers)
	r.startParallelIteratorWorkers(ctx, cancel, jobs, results, &wg, workers)
	go feedParallelIteratorJobs(ctx, jobs, commands)
	go func() {
		wg.Wait()
		close(results)
	}()

	completed := make([]parallelIteratorResult, 0, len(commands))
	for result := range results {
		completed = append(completed, result)
	}
	sort.Slice(completed, func(i, j int) bool { return completed[i].index < completed[j].index })
	return completed
}

func (r *loopRunner) startParallelIteratorWorkers(
	ctx context.Context,
	cancel context.CancelFunc,
	jobs <-chan parallelIteratorCommand,
	results chan<- parallelIteratorResult,
	wg *sync.WaitGroup,
	workers int,
) {
	for range workers {
		go func() {
			defer wg.Done()
			for job := range jobs {
				dispatchContext := ctx
				if _, contextual := job.command.(ContextCommand); !contextual {
					// Legacy commands cannot be interrupted safely. Let them finish
					// so fail-fast cancellation never leaves orphaned work.
					dispatchContext = context.WithoutCancel(ctx)
				}
				result := dispatchWithMonitorContext(
					dispatchContext, job.command, r.trace, r.params.CommandTimeout,
					r.params.MonitorRecorder, job.context,
				)
				results <- parallelIteratorResult{index: job.index, result: result}
				if iteratorResultHalts(r.iterator.Spec, result) {
					cancel()
				}
			}
		}()
	}
}

func feedParallelIteratorJobs(
	ctx context.Context,
	jobs chan<- parallelIteratorCommand,
	commands []parallelIteratorCommand,
) {
	defer close(jobs)
	for _, command := range commands {
		select {
		case jobs <- command:
		case <-ctx.Done():
			return
		}
	}
}

func iteratorResultHalts(spec ForEachSpec, result Result) bool {
	return signalIn(result.Signal, spec.AbortOn) ||
		(!signalIn(result.Signal, spec.ContinueOn) && effectiveFailure(spec) == ForEachFailFast)
}
