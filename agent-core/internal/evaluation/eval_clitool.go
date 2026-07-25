// Copyright (c) 2026 Nokia. All rights reserved.

package evaluation

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/execute"
)

// runAgentCmd executes a harness binary as a subprocess with flag
// propagation from the parent's span context and budget.
type runAgentCmd struct {
	pc  *PointContext
	ctx context.Context
}

func (c *runAgentCmd) Name() string { return "run_agent" }
func (c *runAgentCmd) Undo(prior core.Result) core.Result {
	return (&evaluatorReceiptCmd{
		inner: c, point: c.pc,
		boundary: "harness child process and point workspace require compensation",
	}).Undo(prior)
}

func (c *runAgentCmd) Execute() core.Result {
	pc := c.pc
	absTrace, _ := filepath.Abs(pc.TracePath)
	if pc.ProfilePath == "" {
		err := fmt.Errorf("run_agent: profile path is required")
		return core.Result{CommandName: c.Name(), Signal: core.CommandError, Err: err, Output: err.Error()}
	}

	result := execute.RunAgent(c.ctx, execute.Config{
		Binary:      pc.Harness.Binary,
		Profile:     pc.ProfilePath,
		Directory:   pc.PointDir,
		OTelLogFile: absTrace,
		Timeout:     pc.Timeout,
	})
	return c.recordResult(result)
}

func (c *runAgentCmd) recordResult(result *execute.Result) core.Result {
	pc := c.pc
	pc.Duration = result.Duration
	pc.ExitCode = result.ExitCode
	pc.TimedOut = result.TimedOut

	if err := os.WriteFile(pc.ResultPath, []byte(result.Stdout), 0o644); err != nil {
		err = fmt.Errorf("run_agent: write result artifact %q: %w", pc.ResultPath, err)
		return core.Result{
			CommandName: c.Name(),
			Signal:      core.CommandError,
			Err:         err,
			Output:      err.Error(),
			Cost:        core.Cost{Duration: pc.Duration},
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
		Output:      result.Stdout,
		Cost:        core.Cost{Duration: pc.Duration},
	}
}
