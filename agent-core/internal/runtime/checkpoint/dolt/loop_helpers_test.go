// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package doltcheckpoint

import (
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

type fakeCmd struct {
	name   string
	signal core.Signal
}

func (f *fakeCmd) Name() string { return f.name }

func (f *fakeCmd) Execute() core.Result {
	return core.Result{Signal: f.signal, CommandName: f.name}
}

func (f *fakeCmd) Undo(_ core.Result) core.Result { return core.NoopUndo(f.name) }

type fakeBuilder struct {
	name   string
	signal core.Signal
}

func (f *fakeBuilder) Build(_ core.Result) core.Command {
	return &fakeCmd{name: f.name, signal: f.signal}
}

type staticBuilder struct {
	cmd core.Command
}

func (s *staticBuilder) Build(_ core.Result) core.Command { return s.cmd }

func simpleLoopParams(tr tracing.Tracer) core.LoopParams {
	reg := core.NewRegistry()
	reg.Register(core.ToolSpec{Name: "step_a", Visibility: core.Internal}, &fakeBuilder{name: "step_a", signal: core.Signal("Done")})
	reg.Register(core.ToolSpec{Name: "step_b", Visibility: core.Internal}, &fakeBuilder{name: "step_b", signal: core.Signal("TaskCompleted")})

	bA, _ := reg.Resolve("step_a")
	bB, _ := reg.Resolve("step_b")

	table := core.TransitionTable{
		{State: "Start", Signal: core.Seed}: {
			NextState: "Working",
			Action:    func(r core.Result) core.Command { return bA.Build(r) },
		},
		{State: "Working", Signal: core.Signal("Done")}: {
			NextState: "Working",
			Action:    func(r core.Result) core.Command { return bB.Build(r) },
		},
		{State: "Working", Signal: core.Signal("TaskCompleted")}: {
			NextState: "Finished",
		},
		{State: "Working", Signal: core.BudgetExhausted}: {
			NextState: "OverBudget",
		},
		{State: "Working", Signal: core.CommandError}: {
			NextState: "Broken",
		},
	}

	terminal := func(s core.State) bool {
		return s == "Finished" || s == "OverBudget" || s == "Broken"
	}

	return core.LoopParams{
		InitialState: "Start",
		Registry:     reg,
		Table:        table,
		IsTerminal:   terminal,
		Trace:        tr,
		Budget:       core.Budget{MaxIterations: 100},
		Hooks: core.LoopHooks{
			TerminalStatus: func(s core.State) core.RunStatus {
				switch s {
				case "Finished":
					return core.StatusSucceeded
				case "OverBudget":
					return core.StatusBudgetExceeded
				default:
					return core.StatusFailed
				}
			},
			TaskCompletedSignal: core.Signal("TaskCompleted"),
		},
	}
}

func suspendLoopParams(tr tracing.Tracer, builder core.Builder) core.LoopParams {
	reg := core.NewRegistry()
	reg.Register(core.ToolSpec{Name: "suspend", Visibility: core.Internal}, builder)
	b, _ := reg.Resolve("suspend")
	return core.LoopParams{
		InitialState: "Start",
		Registry:     reg,
		Table: core.TransitionTable{
			{State: "Start", Signal: core.Seed}: {
				NextState: "AwaitingApproval",
				Action:    func(r core.Result) core.Command { return b.Build(r) },
			},
			{State: "AwaitingApproval", Signal: core.CommandError}: {
				NextState: "Failed",
			},
		},
		IsTerminal: func(s core.State) bool { return s == "Failed" },
		Trace:      tr,
		Budget:     core.Budget{MaxIterations: 10},
	}
}

func resumeLoopParams() core.LoopParams {
	reg := core.NewRegistry()
	reg.Register(core.ToolSpec{Name: "finish", Visibility: core.Internal}, &fakeBuilder{name: "finish", signal: core.TaskCompleted})
	builder, _ := reg.Resolve("finish")
	return core.LoopParams{
		InitialState:  "Start",
		InitialSignal: core.Approved,
		Registry:      reg,
		Table: core.TransitionTable{
			{State: "AwaitingApproval", Signal: core.Approved}: {
				NextState: "Finishing",
				Action:    func(r core.Result) core.Command { return builder.Build(r) },
			},
			{State: "Finishing", Signal: core.TaskCompleted}: {
				NextState: "Finished",
			},
		},
		IsTerminal: func(s core.State) bool { return s == "Finished" },
		Trace:      tracing.NoopTracer{},
		Budget:     core.Budget{MaxIterations: 10},
		Hooks: core.LoopHooks{
			TaskCompletedSignal: core.TaskCompleted,
			TerminalStatus: func(s core.State) core.RunStatus {
				if s == "Finished" {
					return core.StatusSucceeded
				}
				return core.StatusFailed
			},
		},
	}
}
