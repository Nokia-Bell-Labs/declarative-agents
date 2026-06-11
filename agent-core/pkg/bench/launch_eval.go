// Copyright (c) 2026 Nokia. All rights reserved.

package bench

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/stl"
)

// LaunchEvalConfig holds the child agent invocation parameters
// for launch_eval, populated from tool declaration YAML config.
type LaunchEvalConfig struct {
	Machine          string
	Tools            string
	ToolDeclarations []string
}

// LaunchEvalBuilder creates launchEvalCmd instances.
type LaunchEvalBuilder struct {
	BS     *BenchState
	Config LaunchEvalConfig
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
	bs     *BenchState
	res    core.Result
	config LaunchEvalConfig
}

func (c *launchEvalCmd) Name() string { return "launch_eval" }

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
	if suitePath == "" {
		return core.Result{
			Signal:      EvalFailed,
			Err:         fmt.Errorf("launch_eval: missing suite path in action config"),
			Output:      "missing suite path",
			CommandName: "launch_eval",
		}
	}

	agentBin := agentBinaryPath()

	args := []string{
		"--machine", c.config.Machine,
		"--tools", c.config.Tools,
	}
	for _, decl := range c.config.ToolDeclarations {
		args = append(args, "--tools-declaration", decl)
	}
	args = append(args, "--input", suitePath)

	if outputDir, ok := action.Config["output_dir"].(string); ok && outputDir != "" {
		args = append(args, "--output", outputDir)
	}

	cmd := exec.Command(agentBin, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return core.Result{
			Signal:      EvalFailed,
			Err:         err,
			Output:      fmt.Sprintf("eval failed: %v", err),
			CommandName: "launch_eval",
		}
	}

	return core.Result{
		Signal:      EvalCompleted,
		Output:      fmt.Sprintf("eval completed for suite %s", suitePath),
		CommandName: "launch_eval",
	}
}

func agentBinaryPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "agent"
	}
	return filepath.Join(filepath.Dir(exe), "agent")
}

// LaunchEvalFactory returns a BuiltinFactory that reads child agent
// invocation parameters from tool declaration config.
func LaunchEvalFactory(bs *BenchState) stl.BuiltinFactory {
	return func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
		var parsed stl.ChildAgentConfig
		if err := stl.DecodeToolConfig(def, &parsed); err != nil {
			return nil, err
		}
		cfg := LaunchEvalConfig{
			Machine:          "configs/evaluator/machine.yaml",
			Tools:            "configs/evaluator/tools.yaml",
			ToolDeclarations: []string{"configs/tools/builtin.yaml"},
		}
		if parsed.Machine != "" {
			cfg.Machine = parsed.Machine
		}
		if parsed.Tools != "" {
			cfg.Tools = parsed.Tools
		}
		if len(parsed.ToolDeclarations) > 0 {
			cfg.ToolDeclarations = parsed.ToolDeclarations
		}
		return &LaunchEvalBuilder{BS: bs, Config: cfg}, nil
	}
}
