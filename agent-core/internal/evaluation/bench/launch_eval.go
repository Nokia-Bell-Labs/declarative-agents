// Copyright (c) 2026 Nokia. All rights reserved.

package bench

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/support/execute"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/catalog"
	toolregistry "gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/registry"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/undo"
)

// LaunchEvalBuilder creates launchEvalCmd instances.
type LaunchEvalBuilder struct {
	BS     *BenchState
	Config execute.Config
}

func (b *LaunchEvalBuilder) Build(res core.Result) core.Command {
	if b.BS == nil {
		return &failCmd{err: fmt.Errorf("launch_eval: BenchState not initialized")}
	}
	return &launchEvalCmd{
		bs:     b.BS,
		res:    res,
		config: b.Config,
	}
}

// launchEvalCmd spawns the agent with a configured machine and tools
// as a subprocess and blocks until it completes. The suite path
// comes from the user action config submitted through the web UI.
type launchEvalCmd struct {
	bs        *BenchState
	res       core.Result
	config    execute.Config
	suitePath string
	outputDir string
}

func (c *launchEvalCmd) Name() string { return "launch_eval" }
func (c *launchEvalCmd) Undo() core.Result {
	err := fmt.Errorf("undo launch_eval requires child evaluator artifact compensation")
	return core.Result{Signal: core.CommandError, CommandName: c.Name(), Output: err.Error(), Err: err}
}
func (c *launchEvalCmd) UndoMemento() (core.UndoMemento, error) {
	payload := undo.BoundaryCompensationPayload{BoundaryCompensation: undo.BoundaryCompensation{
		Strategy:       "child_eval_artifact_compensation",
		Reason:         "launch_eval spawns an evaluator child agent",
		Requires:       []string{"child_history", "artifact_dir"},
		ChildProfile:   c.config.Profile,
		ArtifactPaths:  []string{c.outputDir},
		WorkspacePaths: []string{c.suitePath},
	}}
	memento, err := core.NewUndoMemento(c.Name(), core.UndoMementoCompensatable, payload)
	if err != nil {
		return core.UndoMemento{}, err
	}
	memento.Description = "compensate evaluator child artifacts or restore child workspace"
	return memento, nil
}

func (c *launchEvalCmd) Execute() core.Result {
	var action UserAction
	if err := json.Unmarshal([]byte(c.res.Output), &action); err != nil {
		return core.Result{
			Signal:      EvalFailed,
			Err:         fmt.Errorf("launch_eval: parse action: %w", err),
			Output:      fmt.Sprintf("failed to parse action: %v", err),
			CommandName: "launch_eval",
		}
	}

	suitePath, _ := action.Config["suite"].(string)
	c.suitePath = suitePath
	if suitePath == "" {
		return core.Result{
			Signal:      EvalFailed,
			Err:         fmt.Errorf("launch_eval: missing suite path in action config"),
			Output:      "missing suite path",
			CommandName: "launch_eval",
		}
	}

	cfg := c.config
	cfg.Request = suitePath
	if outputDir, ok := action.Config["output_dir"].(string); ok && outputDir != "" {
		c.outputDir = outputDir
		cfg.Output = outputDir
	}

	result := execute.RunAgent(context.Background(), cfg)

	if !result.Success() {
		return core.Result{
			Signal:      EvalFailed,
			Err:         fmt.Errorf("eval exited %d", result.ExitCode),
			Output:      fmt.Sprintf("eval failed (exit %d)", result.ExitCode),
			CommandName: "launch_eval",
		}
	}

	return core.Result{
		Signal:      EvalCompleted,
		Output:      fmt.Sprintf("eval completed for suite %s", suitePath),
		CommandName: "launch_eval",
		Cost:        core.Cost{Duration: result.Duration},
	}
}

func agentBinaryPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "agent"
	}
	return filepath.Join(filepath.Dir(exe), "agent")
}

// LaunchEvalFactory returns a registry.BuiltinFactory that reads child agent
// invocation parameters from tool declaration config.
func LaunchEvalFactory(bs *BenchState) toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		var parsed catalog.ChildAgentConfig
		if err := catalog.DecodeToolConfig(def, &parsed); err != nil {
			return nil, err
		}
		if err := catalog.ValidateChildAgentConfig(def.Name, parsed); err != nil {
			return nil, err
		}
		cfg := execute.Config{
			Binary:  agentBinaryPath(),
			Profile: parsed.Profile,
		}
		return &LaunchEvalBuilder{BS: bs, Config: cfg}, nil
	}
}
