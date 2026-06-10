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

// LaunchEvalBuilder creates launchEvalCmd instances.
type LaunchEvalBuilder struct {
	BS *BenchState
}

func (b *LaunchEvalBuilder) Build(res core.Result) core.Command {
	if b.BS == nil {
		return &failCmd{err: fmt.Errorf("launch_eval: BenchState not initialized")}
	}
	return &launchEvalCmd{
		bs:  b.BS,
		res: res,
	}
}

// launchEvalCmd spawns `agent eval <suite>` as a subprocess and
// blocks until it completes. The suite path comes from the user
// action config submitted through the web UI.
type launchEvalCmd struct {
	bs  *BenchState
	res core.Result
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

	args := []string{"eval", suitePath}
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

// LaunchEvalFactory returns a BuiltinFactory that creates
// LaunchEvalBuilder instances.
func LaunchEvalFactory(bs *BenchState) stl.BuiltinFactory {
	return func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
		return &LaunchEvalBuilder{BS: bs}, nil
	}
}
