// Copyright (c) 2026 Nokia. All rights reserved.

package core

// resumeLoopParams builds a minimal machine that finishes one step after an
// approval, shared by the port-based resume tests (resume_port_test.go).
func resumeLoopParams() LoopParams {
	reg := NewRegistry()
	reg.Register(ToolSpec{Name: "finish", Visibility: Internal}, &fakeBuilder{name: "finish", signal: TaskCompleted})
	builder, _ := reg.Resolve("finish")
	return LoopParams{
		InitialState: "Start",
		Registry:     reg,
		Table: TransitionTable{
			{State: "AwaitingApproval", Signal: Approved}: {
				NextState: "Finishing",
				Action:    func(r Result) Command { return builder.Build(r) },
			},
			{State: "Finishing", Signal: TaskCompleted}: {
				NextState: "Finished",
			},
		},
		IsTerminal: func(s State) bool { return s == "Finished" },
		Trace:      &loopRecorder{},
		Budget:     Budget{MaxIterations: 10},
		Hooks: LoopHooks{
			TaskCompletedSignal: TaskCompleted,
			TerminalStatus: func(s State) RunStatus {
				if s == "Finished" {
					return StatusSucceeded
				}
				return StatusFailed
			},
		},
	}
}
