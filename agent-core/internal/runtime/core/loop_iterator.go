// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package core

import (
	"encoding/json"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
)

func (r *loopRunner) prepareIterator(
	signal Signal,
	next State,
	cmd Command,
	label string,
) (State, Command, string) {
	transition := machineTransition(r.params.MachineSpec, r.state, signal)
	if transition == nil || transition.ForEach == nil {
		return next, cmd, label
	}
	frame, err := r.newIteratorSnapshot(*transition)
	if err != nil {
		return next, iteratorResultCommand{result: Result{
			Signal: CommandError, CommandName: "for_each", Output: err.Error(), Err: err,
		}}, label
	}
	if len(frame.Items) == 0 {
		result := iteratorJoinResult(frame)
		return State(frame.Spec.Join.Next), iteratorResultCommand{result: result}, frame.Spec.Join.Label
	}
	r.iterator = frame
	r.trace.Event("for_each.started",
		attribute.String("as", frame.Spec.As),
		attribute.Int("items", len(frame.Items)),
	)
	return next, cmd, label
}

func machineTransition(spec *MachineSpec, state State, signal Signal) *TransitionSpec {
	if spec == nil {
		return nil
	}
	for i := range spec.Transitions {
		tr := &spec.Transitions[i]
		if tr.State == string(state) && tr.Signal == string(signal) {
			return tr
		}
	}
	return nil
}

func (r *loopRunner) newIteratorSnapshot(tr TransitionSpec) (*IteratorSnapshot, error) {
	value, err := ResolveFromSelector(NewCommandStateView(r.execution), tr.ForEach.Items)
	if err != nil {
		return nil, fmt.Errorf("for_each items: %w", err)
	}
	items, ok := value.([]interface{})
	if !ok {
		return nil, fmt.Errorf("for_each items selector %q did not resolve to an array", tr.ForEach.Items)
	}
	raw, err := marshalIteratorItems(items)
	if err != nil {
		return nil, err
	}
	return &IteratorSnapshot{
		TransitionState: r.state, TransitionSignal: Signal(tr.Signal),
		BodyState: State(tr.Next), Action: tr.Action, Label: tr.Label,
		Spec: *tr.ForEach, Items: raw,
	}, nil
}

func marshalIteratorItems(items []interface{}) ([]json.RawMessage, error) {
	raw := make([]json.RawMessage, 0, len(items))
	for i, item := range items {
		binding := item
		if _, object := item.(map[string]interface{}); !object {
			binding = map[string]interface{}{"value": item}
		}
		data, err := json.Marshal(binding)
		if err != nil {
			return nil, fmt.Errorf("for_each item %d: %w", i, err)
		}
		raw = append(raw, data)
	}
	return raw, nil
}

func (r *loopRunner) iteratorBindings() map[string]string {
	if r.iterator == nil || r.iterator.NextIndex >= len(r.iterator.Items) {
		return nil
	}
	return map[string]string{
		r.iterator.Spec.As: string(r.iterator.Items[r.iterator.NextIndex]),
	}
}

func (r *loopRunner) recordIteratorOutcome() {
	frame := r.iterator
	if frame == nil || frame.NextIndex >= len(frame.Items) {
		return
	}
	frame.Outcomes = append(frame.Outcomes, IteratorOutcome{
		Index: frame.NextIndex, Input: cloneRawMessage(frame.Items[frame.NextIndex]),
		CommandName: r.result.CommandName, Result: DigestResult(r.result),
	})
	if signalIn(r.signal, frame.Spec.AbortOn) ||
		(!signalIn(r.signal, frame.Spec.ContinueOn) && effectiveFailure(frame.Spec) == ForEachFailFast) {
		frame.Halted = true
	}
	frame.NextIndex++
}

func (r *loopRunner) doneIterator() bool {
	if !r.iterator.Halted && r.iterator.NextIndex < len(r.iterator.Items) {
		if effectiveForEachMode(r.iterator.Spec) == ForEachParallel {
			return r.dispatchParallelIterator()
		}
		return r.dispatchIteratorItem()
	}
	return r.dispatchIteratorJoin()
}

func (r *loopRunner) dispatchIteratorItem() bool {
	frame := r.iterator
	builder, ok := r.params.Registry.Resolve(frame.Action)
	if !ok {
		r.result = Result{Signal: CommandError, CommandName: "for_each", Err: fmt.Errorf("iterator action %q is unavailable", frame.Action)}
		frame.Halted = true
		return false
	}
	input := r.result
	input.State = frame.BodyState
	r.iteration++
	r.trace.Event("for_each.item",
		attribute.String("as", frame.Spec.As),
		attribute.Int("index", frame.NextIndex),
		attribute.Int("items", len(frame.Items)),
	)
	r.dispatch(builder.Build(input), transitionMetricLabels(r.params.MachineSpec, frame.TransitionState, frame.TransitionSignal),
		frame.TransitionSignal, frame.Label, frame.BodyState)
	return r.stopForSuspend()
}

func (r *loopRunner) dispatchIteratorJoin() bool {
	frame := r.iterator
	result := iteratorJoinResult(frame)
	fromState := r.state
	r.state = State(frame.Spec.Join.Next)
	r.iterator = nil
	r.iteration++
	r.trace.Event("for_each.join",
		attribute.Int("items", len(frame.Items)),
		attribute.String("signal", string(result.Signal)),
	)
	r.dispatch(iteratorResultCommand{result: result}, nil, result.Signal, frame.Spec.Join.Label, fromState)
	return r.stopAfterDispatch(r.state)
}

func iteratorJoinResult(frame *IteratorSnapshot) Result {
	succeeded, failed := iteratorCounts(frame)
	signal := Signal(frame.Spec.Join.Signals.AllSuccess)
	switch {
	case len(frame.Items) == 0:
		signal = Signal(frame.Spec.Join.Signals.Empty)
	case failed > 0 && (frame.Halted || effectiveFailure(frame.Spec) == ForEachFailFast):
		signal = Signal(frame.Spec.Join.Signals.Failed)
	case failed == len(frame.Items):
		signal = Signal(frame.Spec.Join.Signals.Failed)
	case failed > 0:
		signal = Signal(frame.Spec.Join.Signals.Partial)
	}
	outcomes := iteratorJoinOutcomes(frame.Outcomes)
	output, _ := json.Marshal(map[string]interface{}{
		"items": outcomes, "succeeded": succeeded, "failed": failed,
		"policy": effectiveFailure(frame.Spec),
	})
	return Result{Signal: signal, CommandName: "for_each.join", Output: string(output)}
}

// iteratorJoinOutcome is a join-only projection. IteratorOutcome remains the
// persisted checkpoint shape, while join output gains a traversable view of a
// digest's already-redacted JSON output.
type iteratorJoinOutcome struct {
	Index       int                      `json:"index"`
	Input       json.RawMessage          `json:"input"`
	CommandName string                   `json:"command_name"`
	Result      iteratorJoinResultDigest `json:"result"`
}

type iteratorJoinResultDigest struct {
	ResultDigest
	StructuredOutput json.RawMessage `json:"structured_output,omitempty"`
}

func iteratorJoinOutcomes(persisted []IteratorOutcome) []iteratorJoinOutcome {
	outcomes := make([]iteratorJoinOutcome, 0, len(persisted))
	for _, outcome := range persisted {
		digest := iteratorJoinResultDigest{ResultDigest: outcome.Result}
		if safeStructuredDigestOutput(outcome.Result) {
			digest.StructuredOutput = cloneRawMessage(json.RawMessage(outcome.Result.Output))
		}
		outcomes = append(outcomes, iteratorJoinOutcome{
			Index: outcome.Index, Input: cloneRawMessage(outcome.Input),
			CommandName: outcome.CommandName, Result: digest,
		})
	}
	return outcomes
}

func safeStructuredDigestOutput(digest ResultDigest) bool {
	return digest.RedactionVersion == OutputRedactionVersion1 &&
		digest.RedactionStatus == OutputRedactionApplied &&
		json.Valid([]byte(digest.Output))
}

func iteratorCounts(frame *IteratorSnapshot) (int, int) {
	succeeded := 0
	for _, outcome := range frame.Outcomes {
		if signalIn(outcome.Result.Signal, frame.Spec.ContinueOn) {
			succeeded++
		}
	}
	return succeeded, len(frame.Outcomes) - succeeded
}

func signalIn(signal Signal, set []string) bool {
	for _, candidate := range set {
		if candidate == string(signal) {
			return true
		}
	}
	return false
}

func cloneIteratorSnapshot(in *IteratorSnapshot) *IteratorSnapshot {
	if in == nil {
		return nil
	}
	out := *in
	out.Items = make([]json.RawMessage, len(in.Items))
	for i := range in.Items {
		out.Items[i] = cloneRawMessage(in.Items[i])
	}
	out.Outcomes = append([]IteratorOutcome(nil), in.Outcomes...)
	return &out
}

func cloneRawMessage(in json.RawMessage) json.RawMessage {
	return append(json.RawMessage(nil), in...)
}

type iteratorResultCommand struct {
	result Result
}

func (c iteratorResultCommand) Name() string       { return c.result.CommandName }
func (c iteratorResultCommand) Execute() Result    { return c.result }
func (c iteratorResultCommand) Undo(Result) Result { return NoopUndo(c.result.CommandName) }
