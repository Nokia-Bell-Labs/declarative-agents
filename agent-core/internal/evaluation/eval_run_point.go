// Copyright (c) 2026 Nokia. All rights reserved.

package evaluation

import (
	"fmt"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolexec "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/exec"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
)

// RunPointBuilder creates runPointCmd instances.
type RunPointBuilder struct {
	ES            *EvalSessionState
	PointRegistry *core.Registry
	Config        catalog.RunPointConfig
}

func (b *RunPointBuilder) Build(_ core.Result) core.Command {
	return &runPointCmd{es: b.ES, pointRegistry: b.PointRegistry, config: b.Config}
}

type runPointCmd struct {
	es            *EvalSessionState
	pointRegistry *core.Registry
	config        catalog.RunPointConfig
	snapshot      evalSessionSnapshot
	hasSnapshot   bool
}

func (c *runPointCmd) Name() string { return "run_point" }
func (c *runPointCmd) Undo(_ core.Result) core.Result {
	return undoEvalSessionSnapshot(c.Name(), c.es, c.snapshot, c.hasSnapshot)
}

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
	c.snapshot = snapshotEvalSession(c.es)
	c.hasSnapshot = true

	agentName := c.config.AgentName
	if agentName == "" {
		agentName = "critic-point"
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
		Registry: c.pointRegistry,
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
		_, _ = fmt.Fprintf(c.es.Stderr, "    ERROR: %v\n", loopErr)
	}

	c.es.RecordPoint(pc)

	status := "PASS"
	if pc.TimedOut {
		status = "TIMEOUT"
	} else if !pc.TestsPassed {
		status = "FAIL"
	}
	_, _ = fmt.Fprintf(c.es.Stderr, "    %s (exit=%d tokens=%d %s)\n",
		status, pc.ExitCode, pc.Tokens, pc.Duration.Round(time.Second))

	return core.Result{
		Signal:      SigPointDone,
		Output:      fmt.Sprintf("%s: %s", pc.PointID, status),
		CommandName: "run_point",
	}
}

// RunPointFactory creates a registry.BuiltinFactory for run_point.
// Nested loop parameters (point_machine, point_tools, agent_name,
// max_iterations, success_state) are read from the tool declaration config block.
func RunPointFactory(es *EvalSessionState) toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		var cfg catalog.RunPointConfig
		if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
			return nil, err
		}
		if err := catalog.ValidateRunPointConfig(def.Name, cfg); err != nil {
			return nil, err
		}
		es.PointMachine = cfg.PointMachine
		pointRegistry, err := buildPointRegistry(&es.EvalState, cfg)
		if err != nil {
			return nil, err
		}
		return &RunPointBuilder{ES: es, PointRegistry: pointRegistry, Config: cfg}, nil
	}
}

func buildPointRegistry(es *EvalState, cfg catalog.RunPointConfig) (*core.Registry, error) {
	selection, err := catalog.LoadToolSelection(cfg.PointTools)
	if err != nil {
		return nil, err
	}
	declarationPaths := make([]string, len(cfg.PointToolDeclarations))
	for i, path := range cfg.PointToolDeclarations {
		declarationPaths[i] = catalog.ResolveConfiguredPath("", path)
	}
	declarations, err := catalog.LoadToolDeclarations(declarationPaths)
	if err != nil {
		return nil, err
	}
	selected, err := catalog.SelectTools(declarations, selection)
	if err != nil {
		return nil, err
	}

	reg := core.NewRegistry()
	builtins := toolregistry.NewBuiltinRegistry()
	RegisterEvalPointFactories(builtins, es)
	pointRoot := func() string {
		if es == nil || es.PC == nil {
			return ""
		}
		return es.PC.PointDir
	}
	execFactory := func(def catalog.ToolDef, _ string) core.Builder {
		return &toolexec.ExecBuilder{Def: def, RootFunc: pointRoot}
	}
	if err := toolregistry.RegisterUnifiedTools(reg, builtins, "", selected, nil, execFactory); err != nil {
		return nil, fmt.Errorf("run_point: register selected point tools: %w", err)
	}
	return reg, nil
}
