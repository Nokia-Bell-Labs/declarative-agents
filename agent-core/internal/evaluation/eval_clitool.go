// Copyright (c) 2026 Nokia. All rights reserved.

package evaluation

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/execute"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/undo"
)

// runAgentCmd executes a harness binary as a subprocess with flag
// propagation from the parent's span context and budget.
type runAgentCmd struct {
	pc          *PointContext
	ctx         context.Context
	snapshot    pointContextSnapshot
	hasSnapshot bool
}

func (c *runAgentCmd) Name() string { return "run_agent" }
func (c *runAgentCmd) Undo(_ core.Result) core.Result {
	result := undoPointContextSnapshot(c.Name(), c.pc, c.snapshot, c.hasSnapshot)
	if result.Signal != core.ToolDone {
		return result
	}
	return undo.BoundaryCompensationUndo(c.Name(), "restore point workspace artifacts and compensate the harness child process")
}

func (c *runAgentCmd) Execute() core.Result {
	pc := c.pc
	c.snapshot = snapshotPointContext(pc)
	c.hasSnapshot = true

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
	pc.Duration = result.Duration
	pc.ExitCode = result.ExitCode
	pc.TimedOut = result.TimedOut

	_ = os.WriteFile(pc.ResultPath, []byte(result.Stdout), 0o644)

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
