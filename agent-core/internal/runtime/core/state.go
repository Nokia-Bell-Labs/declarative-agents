// Copyright (c) 2026 Nokia. All rights reserved.

package core

import "fmt"

// ActionFunc builds the next Command from the previous Result.
type ActionFunc func(Result) Command

// TransitionInput is the lookup key for the transition table.
type TransitionInput struct {
	State  State
	Signal Signal
}

// TransitionValue is the result of a transition table lookup.
// Action is nil for terminal transitions.
type TransitionValue struct {
	NextState State
	Action    ActionFunc
}

// TransitionTable maps state-signal pairs to transitions.
type TransitionTable map[TransitionInput]TransitionValue

// HasState reports whether the machine references the given state, either as the
// source of a transition or as a transition's destination. Resume uses it to
// reject a checkpoint whose restored state no longer exists in the current
// machine (srd025 R6.4, R6.5).
func (t TransitionTable) HasState(s State) bool {
	for in, tv := range t {
		if in.State == s || tv.NextState == s {
			return true
		}
	}
	return false
}

// TerminalFunc returns true for states that should stop the loop.
// Supplied by the domain layer.
type TerminalFunc func(State) bool

// StateMachine holds an immutable transition table and a terminal check.
type StateMachine struct {
	table      TransitionTable
	isTerminal TerminalFunc
}

// NewStateMachine creates a StateMachine from the given table and
// terminal state predicate. The table is captured by reference.
func NewStateMachine(table TransitionTable, isTerminal TerminalFunc) *StateMachine {
	return &StateMachine{table: table, isTerminal: isTerminal}
}

// Step performs a single table lookup and returns the next state and a
// Command built by the matching ActionFunc. If no table entry exists,
// Step returns the failed state and a nil Command.
func (sm *StateMachine) Step(current State, sig Signal, res Result) (State, Command, error) {
	key := TransitionInput{State: current, Signal: sig}
	tv, ok := sm.table[key]
	if !ok {
		return State("Failed"), nil, fmt.Errorf(
			"unhandled state-signal pair: state=%s signal=%s", current, sig,
		)
	}

	if tv.Action == nil {
		return tv.NextState, nil, nil
	}

	cmd := tv.Action(res)
	return tv.NextState, cmd, nil
}

// IsTerminal delegates to the domain-supplied terminal check.
func (sm *StateMachine) IsTerminal(s State) bool {
	return sm.isTerminal(s)
}
