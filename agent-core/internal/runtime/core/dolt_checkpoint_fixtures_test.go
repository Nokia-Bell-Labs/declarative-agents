// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"encoding/json"
	"time"
)

func sampleExecution() Execution {
	ts := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	return Execution{
		{
			Iteration: 1, Timestamp: ts, CommandName: "invoke", Label: "draft",
			FromState: "Start", ToState: "Working", Signal: LLMResponded,
			Result:  checkpointDigest(LLMResponded, "hi", Cost{Duration: 2 * time.Second, TokensIn: 10, TokensOut: 5, Dollars: 0.01}),
			Receipt: `{"file":"a.txt"}`,
		},
		{
			Iteration: 2, Timestamp: ts.Add(time.Second), CommandName: "read",
			FromState: "Working", ToState: "Done", Signal: TaskCompleted,
			Result:  checkpointDigest(TaskCompleted, "done", Cost{TokensIn: 3, TokensOut: 1, Dollars: 0.002}),
			Receipt: "",
		},
	}
}

// threeStepExecution is a three-entry run where every step carries a distinct
// receipt, used to prove revert and terminal reaping across both planes.
func threeStepExecution() Execution {
	ts := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	return Execution{
		{
			Iteration: 1, Timestamp: ts, CommandName: "invoke",
			FromState: "Start", ToState: "Working", Signal: LLMResponded,
			Result:  checkpointDigest(LLMResponded, "s0", Cost{TokensIn: 1}),
			Receipt: `{"step":0}`,
		},
		{
			Iteration: 2, Timestamp: ts.Add(time.Second), CommandName: "read",
			FromState: "Working", ToState: "Working", Signal: LLMResponded,
			Result:  checkpointDigest(LLMResponded, "s1", Cost{TokensIn: 2}),
			Receipt: `{"step":1}`,
		},
		{
			Iteration: 3, Timestamp: ts.Add(2 * time.Second), CommandName: "write",
			FromState: "Working", ToState: "Done", Signal: TaskCompleted,
			Result:  checkpointDigest(TaskCompleted, "s2", Cost{TokensIn: 3}),
			Receipt: `{"step":2}`,
		},
	}
}

func checkpointDigest(signal Signal, output string, cost Cost) ResultDigest {
	return ResultDigest{
		Signal:           signal,
		Output:           output,
		Cost:             cost,
		RedactionVersion: OutputRedactionVersion1,
		RedactionStatus:  OutputRedactionApplied,
	}
}

func samplePosition() Position {
	return Position{
		CurrentState: "Working",
		LastSignal:   LLMResponded,
		Snapshot: AgentSnapshot{
			State:        "Working",
			Signal:       LLMResponded,
			Iteration:    1,
			TokensIn:     10,
			TokensOut:    5,
			TotalCost:    0.01,
			Conversation: json.RawMessage(`[{"role":"user","content":"hi"}]`),
		},
	}
}

// receiptReverser is a receipt-consuming test command: its Undo records the
// opaque receipt it was handed, standing in for a tool that reverses its own
// external effect from the receipt.
type receiptReverser struct{ seen string }

func (r *receiptReverser) Name() string { return "reverser" }

func (r *receiptReverser) Execute() Result { return Result{} }

func (r *receiptReverser) Undo(prior Result) Result {
	r.seen = prior.Receipt
	return NoopUndo(r.Name())
}
