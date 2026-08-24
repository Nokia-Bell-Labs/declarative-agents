// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

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
			Domain:       json.RawMessage(`{"consecutive_parse_errors":2}`),
			Program: ProgramRef{
				Profile: "/profiles/origin/profile.yaml",
				Digest:  "0123456789abcdef",
			},
		},
	}
}
