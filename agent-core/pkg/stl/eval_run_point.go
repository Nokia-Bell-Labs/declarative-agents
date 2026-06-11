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
}

func (b *RunPointBuilder) Build(_ core.Result) core.Command {
	return &runPointCmd{es: b.ES, registry: b.Registry}
}

type runPointCmd struct {
	es       *EvalSessionState
	registry *core.Registry
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

	params := core.LoopParams{
		MachineFile: c.es.PointMachine,
		AgentName:   "evaluator-point",
		Trace:       tracing.NoopTracer{},
		Budget: core.Budget{
			MaxIterations: 20,
		},
		Registry: c.registry,
		Hooks: core.LoopHooks{
			TerminalStatus: func(s core.State) core.RunStatus {
				if s == core.State("Done") {
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
// Config key: point_machine (path to the per-point state machine YAML).
func RunPointFactory(es *EvalSessionState, registry *core.Registry) BuiltinFactory {
	return func(def ToolDef, vars map[string]string) (core.Builder, error) {
		if es.PointMachine == "" {
			if v, ok := def.Config["point_machine"].(string); ok && v != "" {
				es.PointMachine = v
			}
		}
		if es.PointMachine == "" {
			es.PointMachine = "configs/evaluator/point.yaml"
		}
		return &RunPointBuilder{ES: es, Registry: registry}, nil
	}
}
