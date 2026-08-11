// Copyright (c) 2026 Nokia. All rights reserved.

package evaluation

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolexec "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/exec"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/filesystem"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
)

// RunPointBuilder creates runPointCmd instances.
type RunPointBuilder struct {
	ES            *EvalSessionState
	PointRegistry *core.Registry
	Config        catalog.RunPointConfig
}

func (b *RunPointBuilder) Build(_ core.Result) core.Command {
	cmd := &runPointCmd{es: b.ES, pointRegistry: b.PointRegistry, config: b.Config}
	return &evaluatorReceiptCmd{
		inner:   cmd,
		session: b.ES, boundary: "nested point machine history and point workspace require rollback",
		boundaryMetadata: func() any { return cmd.runResult },
	}
}

func (b *RunPointBuilder) BuildReverser() core.Command {
	return &evaluatorReceiptCmd{
		inner:   &runPointCmd{es: b.ES, pointRegistry: b.PointRegistry, config: b.Config},
		session: b.ES, boundary: "nested point machine history and point workspace require rollback",
	}
}

type runPointCmd struct {
	es            *EvalSessionState
	pointRegistry *core.Registry
	config        catalog.RunPointConfig
	runResult     core.RunResult
	commandState  core.CommandStateView
}

func (c *runPointCmd) Name() string { return "run_point" }
func (c *runPointCmd) SetCommandState(view core.CommandStateView) {
	c.commandState = view
}
func (c *runPointCmd) Undo(prior core.Result) core.Result {
	return (&evaluatorReceiptCmd{
		inner: c, session: c.es,
		boundary: "nested point machine history and point workspace require rollback",
	}).Undo(prior)
}

func (c *runPointCmd) Execute() core.Result {
	if c.commandState != nil {
		if err := c.bindPointContext(); err != nil {
			return core.Result{
				Signal: core.CommandError, Err: err, Output: err.Error(), CommandName: c.Name(),
			}
		}
	}
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

	tracer := c.es.Tracer
	if tracer == nil {
		tracer = tracing.NoopTracer{}
	}
	params := core.LoopParams{
		MachineFile: c.es.PointMachine,
		AgentName:   agentName,
		Trace:       tracer,
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

	runResult, loopErr := core.Loop(params, c.es.Ctx)
	c.runResult = runResult
	if loopErr != nil {
		_, _ = fmt.Fprintf(c.es.Stderr, "    ERROR: %v\n", loopErr)
		c.es.RecordPoint(pc)
		return core.Result{
			Signal:      core.CommandError,
			Err:         fmt.Errorf("run_point: nested point loop: %w", loopErr),
			Output:      loopErr.Error(),
			CommandName: "run_point",
		}
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

	output, err := json.Marshal(map[string]interface{}{
		"point_id": pc.PointID, "machine": c.config.PointMachine, "status": status,
		"terminal_state": string(runResult.FinalState),
		"elapsed_ms":     runResult.Duration.Milliseconds(), "artifact_dir": pc.PointDir,
	})
	if err != nil {
		return core.Result{
			Signal: core.CommandError, Err: err, Output: err.Error(), CommandName: c.Name(),
		}
	}
	return core.Result{
		Signal:      SigPointDone,
		Output:      string(output),
		CommandName: "run_point",
	}
}

func (c *runPointCmd) bindPointContext() error {
	data, ok := c.commandState.Lookup("point")
	if !ok {
		return fmt.Errorf("run_point: bind iterator point: command-state label %q not found", "point")
	}
	var input evalPointInput
	if err := json.Unmarshal([]byte(data), &input); err != nil {
		return fmt.Errorf("run_point: decode iterator point: %w", err)
	}
	c.es.PC = &PointContext{
		SessionDir:  c.es.SessionDir,
		PointID:     input.PointID,
		Sample:      input.Sample,
		Harness:     input.Harness,
		Model:       input.Model,
		ProfilePath: input.ProfilePath,
		CoreRoot:    c.es.CoreRoot,
		GridPoint:   input.GridPoint,
		Rep:         input.Rep,
		Timeout:     c.es.timeout,
		LLMTimeout:  c.es.llmTimeout,
		OllamaURL:   c.es.ollamaURL,
		Stderr:      c.es.Stderr,
	}
	_, _ = fmt.Fprintf(c.es.Stderr, "  → %s\n", input.PointID)
	return nil
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
	builtins, execFactory := pointToolFactories(es)
	if err := toolregistry.RegisterUnifiedTools(reg, builtins, "", selected, nil, execFactory); err != nil {
		return nil, fmt.Errorf("run_point: register selected point tools: %w", err)
	}
	return reg, nil
}

// pointToolFactories builds the point-registry builtin registry and exec
// factory, both rooted at the current point workspace resolved at dispatch.
func pointToolFactories(es *EvalState) (*toolregistry.BuiltinRegistry, toolregistry.ExecBuilderFactory) {
	builtins := toolregistry.NewBuiltinRegistry()
	RegisterEvalPointFactories(builtins, es)
	pointRoot := func() string {
		if es == nil || es.PC == nil {
			return ""
		}
		return es.PC.PointDir
	}
	// The result-artifact write is a declared machine transition (GH-1378):
	// run_agent emits path/content parameters and the point machine dispatches
	// the generic write word rooted at the current point workspace.
	builtins.Register("file_write", func(def catalog.ToolDef, _ map[string]string) (core.Builder, error) {
		return &filesystem.WriteBuilder{RootFunc: pointRoot, Metrics: def.Metrics}, nil
	})
	execFactory := func(def catalog.ToolDef, _ string) core.Builder {
		return &toolexec.ExecBuilder{Def: def, RootFunc: pointRoot}
	}
	return builtins, execFactory
}
