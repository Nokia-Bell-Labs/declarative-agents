// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"fmt"
	"time"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/tracing"
)

// RunPointBuilder creates runPointCmd instances.
type RunPointBuilder struct {
	ES       *EvalSessionState
	Registry *core.Registry
	Config   RunPointConfig
}

func (b *RunPointBuilder) Build(_ core.Result) core.Command {
	return &runPointCmd{es: b.ES, registry: b.Registry, config: b.Config}
}

type runPointCmd struct {
	es       *EvalSessionState
	registry *core.Registry
	config   RunPointConfig
}

func (c *runPointCmd) Name() string { return "run_point" }

func (c *runPointCmd) Execute() core.Result {
	pc := c.es.PC
	if pc == nil {
		return core.Result{
			Signal:      core.CommandError,
			Err:         fmt.Errorf("run_point: no current point"),
			Output:      "no current point",
			CommandName: "run_point",
		}
	}

	agentName := c.config.AgentName
	if agentName == "" {
		agentName = "evaluator-point"
	}
	maxIter := c.config.MaxIterations
	if maxIter <= 0 {
		maxIter = 20
	}
	successState := c.config.SuccessState
	if successState == "" {
		successState = "Done"
	}

	params := core.LoopParams{
		MachineFile: c.es.PointMachine,
		AgentName:   agentName,
		Trace:       tracing.NoopTracer{},
		Budget: core.Budget{
			MaxIterations: maxIter,
		},
		Registry: c.registry,
		Hooks: core.LoopHooks{
			TerminalStatus: func(s core.State) core.RunStatus {
				if s == core.State(successState) {
					return core.StatusSucceeded
				}
				return core.StatusFailed
			},
		},
	}

	_, loopErr := core.Loop(params, c.es.Ctx)
	if loopErr != nil {
		fmt.Fprintf(c.es.Stderr, "    ERROR: %v\n", loopErr)
	}

	c.es.RecordPoint(pc)

	status := "PASS"
	if pc.TimedOut {
		status = "TIMEOUT"
	} else if !pc.TestsPassed {
		status = "FAIL"
	}
	fmt.Fprintf(c.es.Stderr, "    %s (exit=%d tokens=%d %s)\n",
		status, pc.ExitCode, pc.Tokens, pc.Duration.Round(time.Second))

	return core.Result{
		Signal:      SigPointDone,
		Output:      fmt.Sprintf("%s: %s", pc.PointID, status),
		CommandName: "run_point",
	}
}

// RunPointFactory creates a BuiltinFactory for run_point.
// Nested loop parameters (point_machine, agent_name, max_iterations,
// success_state) are read from the tool declaration config block.
func RunPointFactory(es *EvalSessionState, registry *core.Registry) BuiltinFactory {
	return func(def ToolDef, vars map[string]string) (core.Builder, error) {
		var cfg RunPointConfig
		if err := DecodeToolConfig(def, &cfg); err != nil {
			return nil, err
		}
		if es.PointMachine == "" && cfg.PointMachine != "" {
			es.PointMachine = cfg.PointMachine
		}
		if es.PointMachine == "" {
			es.PointMachine = "configs/evaluator/point.yaml"
		}
		return &RunPointBuilder{ES: es, Registry: registry, Config: cfg}, nil
	}
}
