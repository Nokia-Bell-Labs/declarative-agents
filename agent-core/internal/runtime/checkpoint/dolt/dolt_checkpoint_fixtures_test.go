// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package doltcheckpoint

import (
	"encoding/json"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func sampleExecution() core.Execution {
	ts := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	return core.Execution{
		{
			Iteration: 1, Timestamp: ts, CommandName: "invoke", Label: "draft",
			FromState: "Start", ToState: "Working", Signal: core.LLMResponded,
			Result:  checkpointDigest(core.LLMResponded, "hi", core.Cost{Duration: 2 * time.Second, TokensIn: 10, TokensOut: 5, Dollars: 0.01}),
			Receipt: `{"file":"a.txt"}`,
		},
		{
			Iteration: 2, Timestamp: ts.Add(time.Second), CommandName: "read",
			FromState: "Working", ToState: "Done", Signal: core.TaskCompleted,
			Result:  checkpointDigest(core.TaskCompleted, "done", core.Cost{TokensIn: 3, TokensOut: 1, Dollars: 0.002}),
			Receipt: "",
		},
	}
}

// threeStepExecution is a three-entry run where every step carries a distinct
// receipt, used to prove revert and terminal reaping across both planes.
func threeStepExecution() core.Execution {
	ts := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	return core.Execution{
		{
			Iteration: 1, Timestamp: ts, CommandName: "invoke",
			FromState: "Start", ToState: "Working", Signal: core.LLMResponded,
			Result:  checkpointDigest(core.LLMResponded, "s0", core.Cost{TokensIn: 1}),
			Receipt: `{"step":0}`,
		},
		{
			Iteration: 2, Timestamp: ts.Add(time.Second), CommandName: "read",
			FromState: "Working", ToState: "Working", Signal: core.LLMResponded,
			Result:  checkpointDigest(core.LLMResponded, "s1", core.Cost{TokensIn: 2}),
			Receipt: `{"step":1}`,
		},
		{
			Iteration: 3, Timestamp: ts.Add(2 * time.Second), CommandName: "write",
			FromState: "Working", ToState: "Done", Signal: core.TaskCompleted,
			Result:  checkpointDigest(core.TaskCompleted, "s2", core.Cost{TokensIn: 3}),
			Receipt: `{"step":2}`,
		},
	}
}

func checkpointDigest(signal core.Signal, output string, cost core.Cost) core.ResultDigest {
	return core.ResultDigest{
		Signal:           signal,
		Output:           output,
		Cost:             cost,
		RedactionVersion: core.OutputRedactionVersion1,
		RedactionStatus:  core.OutputRedactionApplied,
	}
}

func samplePosition() core.Position {
	return core.Position{
		CurrentState: "Working",
		LastSignal:   core.LLMResponded,
		Snapshot: core.AgentSnapshot{
			State:        "Working",
			Signal:       core.LLMResponded,
			Iteration:    1,
			TokensIn:     10,
			TokensOut:    5,
			TotalCost:    0.01,
			Conversation: json.RawMessage(`[{"role":"user","content":"hi"}]`),
			Domain:       json.RawMessage(`{"consecutive_parse_errors":2}`),
			Program: core.ProgramRef{
				Profile: "/profiles/origin/profile.yaml",
				Digest:  "0123456789abcdef",
			},
		},
	}
}

// receiptReverser is a receipt-consuming test command: its Undo records the
// opaque receipt it was handed, standing in for a tool that reverses its own
// external effect from the receipt.
type receiptReverser struct{ seen string }

func (r *receiptReverser) Name() string { return "reverser" }

func (r *receiptReverser) Execute() core.Result { return core.Result{} }

func (r *receiptReverser) Undo(prior core.Result) core.Result {
	r.seen = prior.Receipt
	return core.NoopUndo(r.Name())
}

func redactionCheckpointEntry(secret string) core.Entry {
	return core.Entry{
		CommandName: "fetch",
		Label:       "fetch",
		Result: core.ResultDigest{
			Output:           `{"secret":"` + secret + `","public":"ok"}`,
			RedactionVersion: core.OutputRedactionVersion1,
			RedactedPaths:    []core.OutputRedactionPath{{"secret"}},
			RedactionStatus:  core.OutputRedactionApplied,
		},
		Receipt: `{"opaque":"receipt"}`,
	}
}
