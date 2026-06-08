// Copyright (c) 2026 Nokia. All rights reserved.

package eval

import (
	"context"
	"fmt"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/tracing"
)

// DefaultExperiment returns an ExperimentConfig that reproduces the
// legacy prepare → run → check → collect flow as a state machine.
// The harness binary is injected so the CLI tool knows what to invoke.
func DefaultExperiment(harness Harness) ExperimentConfig {
	return ExperimentConfig{
		Name:         "default",
		InitialState: "Idle",
		Tools: map[string]ExperimentTool{
			"prepare_workspace": {Type: "builtin"},
			"run_agent": {
				Type:      "cli",
				Binary:    harness.Binary,
				FlagsFrom: "harness",
				Propagate: []string{"otel-parent-span", "otel-log-file", "llm-timeout", "max-time"},
			},
			"check_results":  {Type: "builtin"},
			"collect_metrics": {Type: "builtin"},
		},
		States: map[string]ExperimentState{
			"Idle": {
				Transitions: []ExperimentTransition{
					{Signal: "Start", Command: "prepare_workspace", NextState: "Preparing"},
				},
			},
			"Preparing": {
				Transitions: []ExperimentTransition{
					{Signal: "WorkspaceReady", Command: "run_agent", NextState: "Running"},
					{Signal: "CommandError", NextState: "Failed"},
				},
			},
			"Running": {
				Transitions: []ExperimentTransition{
					{Signal: "HarnessFinished", Command: "check_results", NextState: "Checking"},
					{Signal: "HarnessFailed", Command: "check_results", NextState: "Checking"},
					{Signal: "HarnessTimedOut", Command: "check_results", NextState: "Checking"},
				},
			},
			"Checking": {
				Transitions: []ExperimentTransition{
					{Signal: "ResultsCollected", Command: "collect_metrics", NextState: "Collecting"},
					{Signal: "CommandError", NextState: "Failed"},
				},
			},
			"Collecting": {
				Transitions: []ExperimentTransition{
					{Signal: "MetricsCollected", NextState: "Done"},
					{Signal: "CommandError", NextState: "Failed"},
				},
			},
			"Done":   {Terminal: true},
			"Failed": {Terminal: true},
		},
	}
}

// ExperimentToMachineSpec converts an ExperimentConfig into a
// core.MachineSpec suitable for core.BuildTransitionTable.
func ExperimentToMachineSpec(exp ExperimentConfig) core.MachineSpec {
	var states []string
	var terminalStates []string
	signalSet := make(map[string]bool)
	var transitions []core.TransitionSpec

	for name, state := range exp.States {
		states = append(states, name)
		if state.Terminal {
			terminalStates = append(terminalStates, name)
		}
		for _, tr := range state.Transitions {
			signalSet[tr.Signal] = true
			transitions = append(transitions, core.TransitionSpec{
				State:  name,
				Signal: tr.Signal,
				Next:   tr.NextState,
				Action: tr.Command,
			})
		}
	}

	// The initial state needs a Seed transition so core.Loop can enter
	// the machine. Map Seed to the first configured transition.
	if initState, ok := exp.States[exp.InitialState]; ok && len(initState.Transitions) > 0 {
		signalSet[string(core.Seed)] = true
		transitions = append(transitions, core.TransitionSpec{
			State:  exp.InitialState,
			Signal: string(core.Seed),
			Next:   initState.Transitions[0].NextState,
			Action: initState.Transitions[0].Command,
		})
	}

	var signals []string
	for s := range signalSet {
		signals = append(signals, s)
	}

	return core.MachineSpec{
		Name:           exp.Name,
		InitialState:   exp.InitialState,
		States:         states,
		TerminalStates: terminalStates,
		Signals:        signals,
		Transitions:    transitions,
	}
}

// RegisterExperimentTools registers the builtin and CLI tools from an
// experiment config into a core.Registry, wired to the given PointContext.
func RegisterExperimentTools(
	reg *core.Registry,
	exp ExperimentConfig,
	pc *PointContext,
	ctx context.Context,
) error {
	for name, tool := range exp.Tools {
		var builder core.Builder
		switch tool.Type {
		case "builtin":
			b, err := builtinBuilder(name, pc)
			if err != nil {
				return err
			}
			builder = b
		case "cli":
			toolDef := tool
			builder = &cliToolBuilder{pc: pc, ctx: ctx, toolDef: toolDef}
		default:
			return fmt.Errorf("unknown tool type %q for tool %q", tool.Type, name)
		}
		reg.Register(core.ToolSpec{
			Name:       name,
			Visibility: core.Internal,
		}, builder)
	}
	return nil
}

func builtinBuilder(name string, pc *PointContext) (core.Builder, error) {
	switch name {
	case "prepare_workspace":
		return &staticBuilder{cmd: &prepareWorkspaceCmd{pc: pc}}, nil
	case "check_results":
		return &staticBuilder{cmd: &checkResultsCmd{pc: pc}}, nil
	case "collect_metrics":
		return &staticBuilder{cmd: &collectMetricsCmd{pc: pc}}, nil
	case "summarize":
		return &staticBuilder{cmd: &summarizeCmd{pc: pc}}, nil
	default:
		return nil, fmt.Errorf("unknown builtin tool %q", name)
	}
}

// staticBuilder always returns the same command regardless of the
// previous result. Suitable for deterministic (non-LLM) tools.
type staticBuilder struct {
	cmd core.Command
}

func (b *staticBuilder) Build(_ core.Result) core.Command {
	return b.cmd
}

// cliToolBuilder creates runAgentCmd instances.
type cliToolBuilder struct {
	pc      *PointContext
	ctx     context.Context
	toolDef ExperimentTool
}

func (b *cliToolBuilder) Build(_ core.Result) core.Command {
	return &runAgentCmd{
		pc:      b.pc,
		ctx:     b.ctx,
		toolDef: b.toolDef,
	}
}

// RunPoint executes a single evaluation point using an experiment state
// machine driven by core.Loop. Returns the PointContext with populated
// result fields.
func RunPoint(
	ctx context.Context,
	exp ExperimentConfig,
	pc *PointContext,
) (*PointContext, error) {
	spec := ExperimentToMachineSpec(exp)

	reg := core.NewRegistry()
	if err := RegisterExperimentTools(reg, exp, pc, ctx); err != nil {
		return pc, fmt.Errorf("register experiment tools: %w", err)
	}

	table, isTerminal, err := core.BuildTransitionTable(spec, reg, nil)
	if err != nil {
		return pc, fmt.Errorf("build transition table: %w", err)
	}

	params := core.LoopParams{
		InitialState: core.State(exp.InitialState),
		Prompt:       "begin",
		Registry:     reg,
		Table:        table,
		IsTerminal:   isTerminal,
		Trace:        tracing.NoopTracer{},
		AgentName:    "evaluator-point",
		Budget: core.Budget{
			MaxIterations: 20,
		},
		Hooks: core.LoopHooks{
			TerminalStatus: func(s core.State) core.RunStatus {
				if isTerminal(s) && s != core.State("Failed") {
					return core.StatusSucceeded
				}
				return core.StatusFailed
			},
		},
	}

	_, err = core.Loop(params, ctx)
	if err != nil {
		return pc, fmt.Errorf("experiment loop: %w", err)
	}

	return pc, nil
}
