// Copyright (c) 2026 Nokia. All rights reserved.

package eval

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"go.opentelemetry.io/otel/trace"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/telemetry"
)

// runAgentCmd executes a harness binary as a subprocess with flag
// propagation from the parent's span context and budget.
type runAgentCmd struct {
	pc      *PointContext
	ctx     context.Context
	toolDef ExperimentTool
}

func (c *runAgentCmd) Name() string { return "run_agent" }

func (c *runAgentCmd) Execute() core.Result {
	pc := c.pc

	binary := resolveBinaryTemplate(c.toolDef.Binary, pc.Harness)

	absTrace, _ := filepath.Abs(pc.TracePath)
	args := []string{
		"--prompt", pc.Sample.PromptPath,
		"--directory", pc.PointDir,
		"--model", pc.Model,
		"--otel-log-file", absTrace,
	}

	if shouldPropagate(c.toolDef.Propagate, "otel-parent-span") {
		sc := trace.SpanFromContext(c.ctx).SpanContext()
		if tp := telemetry.FormatTraceparent(sc); tp != "" {
			args = append(args, "--otel-parent-span", tp)
		}
	}

	if shouldPropagate(c.toolDef.Propagate, "max-time") && pc.Timeout > 0 {
		args = append(args, "--max-time", fmt.Sprintf("%d", int(pc.Timeout.Seconds())))
	}
	if shouldPropagate(c.toolDef.Propagate, "llm-timeout") && pc.LLMTimeout > 0 {
		args = append(args, "--llm-timeout", fmt.Sprintf("%d", int(pc.LLMTimeout.Seconds())))
	}

	if c.toolDef.FlagsFrom == "harness" {
		for flag, val := range pc.Harness.Flags {
			resolved := resolveTemplate(val, pc.GridPoint)
			if resolved != "" {
				args = append(args, "--"+flag, resolved)
			} else {
				args = append(args, "--"+flag)
			}
		}
	}

	runCtx, cancel := context.WithTimeout(c.ctx, pc.Timeout)
	defer cancel()

	start := time.Now()
	cmd := exec.CommandContext(runCtx, binary, args...)
	output, cmdErr := cmd.CombinedOutput()
	pc.Duration = time.Since(start)

	_ = os.WriteFile(pc.ResultPath, output, 0o644)

	pc.ExitCode = 0
	if cmdErr != nil {
		if runCtx.Err() == context.DeadlineExceeded {
			pc.TimedOut = true
		}
		if exitErr, ok := cmdErr.(*exec.ExitError); ok {
			pc.ExitCode = exitErr.ExitCode()
		} else {
			pc.ExitCode = -1
		}
	}

	sig := SigHarnessFinished
	if pc.TimedOut {
		sig = SigHarnessTimedOut
	} else if pc.ExitCode != 0 {
		sig = SigHarnessFailed
	}

	return core.Result{
		CommandName: c.Name(),
		Signal:      sig,
		Output:      string(output),
		Cost:        core.Cost{Duration: pc.Duration},
	}
}

func resolveBinaryTemplate(tmpl string, harness Harness) string {
	if strings.Contains(tmpl, "{{harness.binary}}") {
		return strings.ReplaceAll(tmpl, "{{harness.binary}}", harness.Binary)
	}
	return tmpl
}

func shouldPropagate(propagate []string, flag string) bool {
	for _, p := range propagate {
		if p == flag {
			return true
		}
	}
	return false
}

func resolveTemplate(template string, point GridPoint) string {
	result := template
	for name, val := range point {
		result = strings.ReplaceAll(result, "${"+name+"}", fmt.Sprintf("%v", val))
	}
	return result
}
