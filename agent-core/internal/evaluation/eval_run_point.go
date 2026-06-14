// Copyright (c) 2026 Nokia. All rights reserved.

package evaluation

import (
	"fmt"
	"time"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/observability/tracing"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/catalog"
	toolregistry "gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/registry"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/undo"
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
func (c *runPointCmd) Undo() core.Result {
	return undoEvalSessionSnapshot(c.Name(), c.es, c.snapshot, c.hasSnapshot)
}
func (c *runPointCmd) UndoMemento() (core.UndoMemento, error) {
	if !c.hasSnapshot {
		return core.UndoMemento{}, fmt.Errorf("%w: no evaluator session snapshot recorded for %s", core.ErrUndoMementoMissing, c.Name())
	}
	memento, err := core.NewUndoMemento(c.Name(), core.UndoMementoCompensatable, struct {
		DomainState struct {
			SuiteName   string `json:"suite_name,omitempty"`
			SessionDir  string `json:"session_dir,omitempty"`
			TotalPoints int    `json:"total_points"`
			GridPoints  int    `json:"grid_points"`
			Started     bool   `json:"started"`
			Exhausted   bool   `json:"exhausted"`
		} `json:"domain_state"`
		BoundaryCompensation undo.BoundaryCompensation `json:"boundary_compensation"`
	}{
		DomainState: struct {
			SuiteName   string `json:"suite_name,omitempty"`
			SessionDir  string `json:"session_dir,omitempty"`
			TotalPoints int    `json:"total_points"`
			GridPoints  int    `json:"grid_points"`
			Started     bool   `json:"started"`
			Exhausted   bool   `json:"exhausted"`
		}{
			SuiteName:   c.snapshot.suite.Name,
			SessionDir:  c.snapshot.sessionDir,
			TotalPoints: c.snapshot.result.TotalPoints,
			GridPoints:  len(c.snapshot.gridPoints),
			Started:     c.snapshot.started,
			Exhausted:   c.snapshot.exhausted,
		},
		BoundaryCompensation: undo.BoundaryCompensation{
			Strategy:     "nested_machine_rollback",
			Reason:       "run_point executes a nested evaluator point machine",
			Requires:     []string{"nested_machine_history", "Workspace"},
			ChildMachine: c.es.PointMachine,
		},
	})
	if err != nil {
		return core.UndoMemento{}, err
	}
	memento.Description = "restores evaluator session state; nested point machine effects may require child rollback or workspace compensation"
	return memento, nil
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
		pointRegistry, err := buildPointRegistry(&es.EvalState, cfg.PointTools)
		if err != nil {
			return nil, err
		}
		return &RunPointBuilder{ES: es, PointRegistry: pointRegistry, Config: cfg}, nil
	}
}

func buildPointRegistry(es *EvalState, selectionPath string) (*core.Registry, error) {
	selection, err := catalog.LoadToolSelection(selectionPath)
	if err != nil {
		return nil, err
	}
	reg := core.NewRegistry()
	for _, name := range selection {
		switch name {
		case "create_point_dir":
			reg.Register(core.ToolSpec{Name: name, Visibility: core.Internal}, &CreatePointDirBuilder{ES: es})
		case "copy_sample_workspace":
			reg.Register(core.ToolSpec{Name: name, Visibility: core.Internal}, &CopySampleWorkspaceBuilder{ES: es})
		case "copy_sample_docs":
			reg.Register(core.ToolSpec{Name: name, Visibility: core.Internal}, &CopySampleDocsBuilder{ES: es})
		case "init_workspace_repo":
			reg.Register(core.ToolSpec{Name: name, Visibility: core.Internal}, &InitWorkspaceRepoBuilder{ES: es})
		case "stage_workspace_baseline":
			reg.Register(core.ToolSpec{Name: name, Visibility: core.Internal}, &StageWorkspaceBaselineBuilder{ES: es})
		case "commit_workspace_baseline":
			reg.Register(core.ToolSpec{Name: name, Visibility: core.Internal}, &CommitWorkspaceBaselineBuilder{ES: es})
		case "dump_config":
			reg.Register(core.ToolSpec{Name: name, Visibility: core.Internal}, &DumpConfigBuilder{ES: es})
		case "run_agent":
			reg.Register(core.ToolSpec{Name: name, Visibility: core.Internal}, &RunAgentBuilder{ES: es})
		case "run_oracle_check":
			reg.Register(core.ToolSpec{Name: name, Visibility: core.Internal}, &RunOracleCheckBuilder{ES: es})
		case "collect_trace_tokens":
			reg.Register(core.ToolSpec{Name: name, Visibility: core.Internal}, &CollectTraceTokensBuilder{ES: es})
		case "check_agent_version":
			reg.Register(core.ToolSpec{Name: name, Visibility: core.Internal}, &CheckAgentVersionBuilder{ES: es})
		case "summarize_point_results":
			reg.Register(core.ToolSpec{Name: name, Visibility: core.Internal}, &SummarizePointResultsBuilder{ES: es})
		case "collect_metrics":
			reg.Register(core.ToolSpec{Name: name, Visibility: core.Internal}, &CollectMetricsBuilder{ES: es})
		default:
			return nil, fmt.Errorf("run_point: unsupported point tool %q in %s", name, selectionPath)
		}
	}
	return reg, nil
}
