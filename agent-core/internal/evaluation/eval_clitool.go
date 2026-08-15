// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package evaluation

import (
	"context"
	"encoding/json"
	"fmt"
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
		CoreRoot:    pc.CoreRoot,
		Directory:   pc.PointDir,
		OTelLogFile: absTrace,
		Timeout:     pc.Timeout,
	})
	return c.recordResult(result)
}

// recordResult projects the child-process outcome into point state and emits
// the result-artifact write parameters for the following builtin write word.
// It no longer performs the filesystem write itself (GH-1378): sibling
// evaluation words emit parameters for a later word rather than doing the side
// effect, so the machine dispatches write with path=result.json and
// content=child stdout.
func (c *runAgentCmd) recordResult(result *execute.Result) core.Result {
	pc := c.pc
	pc.Duration = result.Duration
	pc.ExitCode = result.ExitCode
	pc.TimedOut = result.TimedOut

	sig := SigHarnessFinished
	if pc.TimedOut {
		sig = SigHarnessTimedOut
	} else if pc.ExitCode != 0 {
		sig = SigHarnessFailed
	}

	output, err := json.Marshal(map[string]any{
		"parameters": map[string]string{
			"path":    ArtifactResult,
			"content": result.Stdout,
		},
	})
	if err != nil {
		err = fmt.Errorf("run_agent: encode result write parameters: %w", err)
		return core.Result{
			CommandName: c.Name(),
			Signal:      core.CommandError,
			Err:         err,
			Output:      err.Error(),
			Cost:        core.Cost{Duration: pc.Duration},
		}
	}

	return core.Result{
		CommandName: c.Name(),
		Signal:      sig,
		Output:      string(output),
		Cost:        core.Cost{Duration: pc.Duration},
	}
}
