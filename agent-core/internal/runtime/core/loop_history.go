// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"time"
)

// resolveCheckpoint returns the configured Checkpoint port, or NoopCheckpoint
// when none is injected so disabled-mode execution keeps its current behavior
// (srd035-checkpoint-port R5.1).
func resolveCheckpoint(c Checkpoint) Checkpoint {
	if c == nil {
		return NoopCheckpoint{}
	}
	return c
}

// dispatchPosition builds the resumable Position from loop-owned state. The
// conversation is folded into the snapshot by the domain that owns it (core
// cannot import the llm package); loop-owned code leaves it empty.
func dispatchPosition(state State, signal Signal, iteration int, rr *RunResult) Position {
	return Position{
		CurrentState: state,
		LastSignal:   signal,
		Snapshot: AgentSnapshot{
			State:     state,
			Signal:    signal,
			Iteration: iteration,
			TokensIn:  rr.TokensIn,
			TokensOut: rr.TokensOut,
			TotalCost: rr.TotalCost,
		},
	}
}

// dispatchEntry builds the Execution entry for one completed dispatch. Signal is
// the transition signal that selected the command; label is the optional stable
// address authored on that transition. Receipt is the tool-owned opaque receipt
// carried verbatim from the Result (srd035 R2.4, R3; srd038 R1.6, R2).
func dispatchEntry(iteration int, fromState, toState State, transitionSignal Signal, label string, res Result) Entry {
	return Entry{
		Iteration:   iteration,
		Timestamp:   time.Now().UTC(),
		CommandName: res.CommandName,
		Label:       label,
		FromState:   fromState,
		ToState:     toState,
		Signal:      transitionSignal,
		Result:      digestResult(res),
		Receipt:     res.Receipt,
	}
}

func digestResult(res Result) ResultDigest {
	digest := ResultDigest{
		Signal: res.Signal,
		Output: res.Output,
		Cost:   res.Cost,
	}
	if res.Err != nil {
		digest.Error = res.Err.Error()
	}
	return digest
}
