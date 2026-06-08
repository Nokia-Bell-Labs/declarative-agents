// Copyright (c) 2026 Nokia. All rights reserved.

package eval

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/subprocess"
)

// runAgentCmd executes a harness binary as a subprocess with flag
// propagation from the parent's span context and budget.
type runAgentCmd struct {
	pc  *PointContext
	ctx context.Context
}

func (c *runAgentCmd) Name() string { return "run_agent" }

func (c *runAgentCmd) Execute() core.Result {
	pc := c.pc

	absTrace, _ := filepath.Abs(pc.TracePath)
	args := []string{
		"--prompt", pc.Sample.PromptPath,
		"--directory", pc.PointDir,
		"--otel-log-file", absTrace,
	}

	for flag, val := range pc.Harness.Flags {
		resolved := resolveTemplate(val, pc.GridPoint)
		if resolved != "" {
			args = append(args, "--"+flag, resolved)
		} else {
			args = append(args, "--"+flag)
		}
	}

	var env []string
	env = append(env, subprocess.EnvVar("AGENT_MODEL", pc.Model))
	if pc.Timeout > 0 {
		env = append(env, subprocess.EnvVarInt("AGENT_MAX_TIME", int(pc.Timeout.Seconds())))
	}
	if pc.LLMTimeout > 0 {
		env = append(env, subprocess.EnvVarInt("AGENT_LLM_TIMEOUT", int(pc.LLMTimeout.Seconds())))
	}

	spec := subprocess.Spec{
		Binary:        pc.Harness.Binary,
		Args:          args,
		Env:           env,
		Timeout:       pc.Timeout,
		PropagateOTel: true,
	}

	r := subprocess.Run(c.ctx, spec)
	pc.Duration = r.Duration
	pc.ExitCode = r.ExitCode
	pc.TimedOut = r.TimedOut

	_ = os.WriteFile(pc.ResultPath, []byte(r.Stdout), 0o644)

	sig := SigHarnessFinished
	if pc.TimedOut {
		sig = SigHarnessTimedOut
	} else if pc.ExitCode != 0 {
		sig = SigHarnessFailed
	}

	return core.Result{
		CommandName: c.Name(),
		Signal:      sig,
		Output:      r.Stdout,
		Cost:        core.Cost{Duration: pc.Duration},
	}
}

func resolveTemplate(template string, point GridPoint) string {
	result := template
	for name, val := range point {
		result = strings.ReplaceAll(result, "${"+name+"}", fmt.Sprintf("%v", val))
	}
	return result
}
