// Copyright (c) 2026 Nokia. All rights reserved.

package core

import "fmt"

const (
	ForEachSequential = "sequential"
	ForEachFailFast   = "fail_fast"
	ForEachCollectAll = "collect_all"
)

// ForEachSpec expands one named transition action over an array selected from
// trusted command state. The MVP is deliberately sequential; it is iteration,
// not a general-purpose scripting surface.
type ForEachSpec struct {
	Items      string   `yaml:"items" json:"items"`
	As         string   `yaml:"as" json:"as"`
	Mode       string   `yaml:"mode,omitempty" json:"mode,omitempty"`
	Failure    string   `yaml:"failure,omitempty" json:"failure,omitempty"`
	ContinueOn []string `yaml:"continue_on" json:"continue_on"`
	AbortOn    []string `yaml:"abort_on,omitempty" json:"abort_on,omitempty"`
	Join       JoinSpec `yaml:"join" json:"join"`
}

// JoinSpec defines the state and signal emitted after ordered item dispatches.
type JoinSpec struct {
	Next    string         `yaml:"next" json:"next"`
	Signals JoinSignalSpec `yaml:"signals" json:"signals"`
	Label   string         `yaml:"label,omitempty" json:"label,omitempty"`
}

// JoinSignalSpec maps aggregate outcomes back into the finite-state grammar.
type JoinSignalSpec struct {
	AllSuccess string `yaml:"all_success" json:"all_success"`
	Partial    string `yaml:"partial,omitempty" json:"partial,omitempty"`
	Failed     string `yaml:"failed" json:"failed"`
	Empty      string `yaml:"empty" json:"empty"`
}

func validateForEachSpec(index int, tr TransitionSpec, states, signals map[string]bool) []string {
	if tr.ForEach == nil {
		return nil
	}
	prefix := fmt.Sprintf("transition[%d].for_each", index)
	var errs []string
	errs = append(errs, validateForEachSource(prefix, tr)...)
	errs = append(errs, validateForEachPolicy(prefix, *tr.ForEach)...)
	errs = append(errs, validateForEachJoin(prefix, *tr.ForEach, states, signals)...)
	for _, signal := range append(append([]string{}, tr.ForEach.ContinueOn...), tr.ForEach.AbortOn...) {
		if !signals[signal] {
			errs = append(errs, fmt.Sprintf("%s: signal %q not in signals list", prefix, signal))
		}
	}
	return errs
}

func validateForEachSource(prefix string, tr TransitionSpec) []string {
	var errs []string
	if tr.Action == "" || tr.Action == "$tool" {
		errs = append(errs, prefix+": requires a named action")
	}
	if _, _, ok := ParseFromSelector(tr.ForEach.Items); !ok {
		errs = append(errs, fmt.Sprintf("%s.items: %q is not a $from(label).path selector", prefix, tr.ForEach.Items))
	}
	if !validSelectorLabel(tr.ForEach.As) {
		errs = append(errs, fmt.Sprintf("%s.as: %q is not a valid command-state label", prefix, tr.ForEach.As))
	}
	if len(tr.ForEach.ContinueOn) == 0 {
		errs = append(errs, prefix+".continue_on: at least one signal is required")
	}
	return errs
}

func validateForEachPolicy(prefix string, spec ForEachSpec) []string {
	var errs []string
	if spec.Mode != "" && spec.Mode != ForEachSequential {
		errs = append(errs, fmt.Sprintf("%s.mode: %q is unsupported; bounded parallel iteration is deferred", prefix, spec.Mode))
	}
	if spec.Failure != "" && spec.Failure != ForEachFailFast && spec.Failure != ForEachCollectAll {
		errs = append(errs, fmt.Sprintf("%s.failure: %q must be fail_fast or collect_all", prefix, spec.Failure))
	}
	if effectiveFailure(spec) == ForEachCollectAll && spec.Join.Signals.Partial == "" {
		errs = append(errs, prefix+".join.signals.partial: required for collect_all")
	}
	return errs
}

func validateForEachJoin(prefix string, spec ForEachSpec, states, signals map[string]bool) []string {
	var errs []string
	if !states[spec.Join.Next] {
		errs = append(errs, fmt.Sprintf("%s.join.next: state %q not in states list", prefix, spec.Join.Next))
	}
	if spec.Join.Label != "" && !validSelectorLabel(spec.Join.Label) {
		errs = append(errs, fmt.Sprintf("%s.join.label: %q is not a valid command-state label", prefix, spec.Join.Label))
	}
	for _, joined := range joinSignals(spec.Join.Signals) {
		if joined.signal == "" {
			errs = append(errs, fmt.Sprintf("%s.join.signals.%s: signal is required", prefix, joined.name))
		} else if !signals[joined.signal] {
			errs = append(errs, fmt.Sprintf("%s.join.signals.%s: signal %q not in signals list", prefix, joined.name, joined.signal))
		}
	}
	return errs
}

func validateForEachJoinRouting(
	index int,
	tr TransitionSpec,
	transitions map[TransitionInput]int,
	terminals map[string]int,
) []string {
	if tr.ForEach == nil {
		return nil
	}
	if _, terminal := terminals[tr.ForEach.Join.Next]; terminal {
		return nil
	}
	var errs []string
	for _, joined := range joinSignals(tr.ForEach.Join.Signals) {
		key := TransitionInput{State: State(tr.ForEach.Join.Next), Signal: Signal(joined.signal)}
		if _, ok := transitions[key]; !ok {
			errs = append(errs, fmt.Sprintf(
				"transition[%d].for_each.join: no transition for state %q and signal %q",
				index, tr.ForEach.Join.Next, joined.signal,
			))
		}
	}
	return errs
}

type namedJoinSignal struct {
	name   string
	signal string
}

func joinSignals(spec JoinSignalSpec) []namedJoinSignal {
	signals := []namedJoinSignal{
		{name: "all_success", signal: spec.AllSuccess},
		{name: "failed", signal: spec.Failed},
		{name: "empty", signal: spec.Empty},
	}
	if spec.Partial != "" {
		signals = append(signals, namedJoinSignal{name: "partial", signal: spec.Partial})
	}
	return signals
}

func effectiveFailure(spec ForEachSpec) string {
	if spec.Failure == "" {
		return ForEachFailFast
	}
	return spec.Failure
}

func markForEachSignals(used map[string]bool, spec *ForEachSpec) {
	if spec == nil {
		return
	}
	for _, signal := range spec.ContinueOn {
		used[signal] = true
	}
	for _, signal := range spec.AbortOn {
		used[signal] = true
	}
	for _, joined := range joinSignals(spec.Join.Signals) {
		used[joined.signal] = true
	}
}
