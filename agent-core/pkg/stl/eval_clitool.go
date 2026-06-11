// Copyright (c) 2026 Nokia. All rights reserved.

package stl

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

func (c *runAgentCmd) Name() string      { return "run_agent" }
func (c *runAgentCmd) Undo() core.Result { return core.NoopUndo(c.Name()) }

func (c *runAgentCmd) Execute() core.Result {
	pc := c.pc

	absTrace, _ := filepath.Abs(pc.TracePath)
	args := []string{
		"--directory", pc.PointDir,
		"--otel-log-file", absTrace,
		"--model", pc.Model,
	}

	for flag, val := range pc.Harness.Flags {
		switch v := val.(type) {
		case string:
			resolved := resolveTemplate(v, pc.GridPoint)
			if resolved != "" {
				args = append(args, "--"+flag, resolved)
			} else {
				args = append(args, "--"+flag)
			}
		case []interface{}:
			for _, elem := range v {
				s := fmt.Sprintf("%v", elem)
				resolved := resolveTemplate(s, pc.GridPoint)
				args = append(args, "--"+flag, resolved)
			}
		default:
			args = append(args, "--"+flag, fmt.Sprintf("%v", val))
		}
	}

	spec := subprocess.Spec{
		Binary:        pc.Harness.Binary,
		Args:          args,
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
