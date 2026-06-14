// Copyright (c) 2026 Nokia. All rights reserved.

package evaluation

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/support/subprocess"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/undo"
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
func (c *runAgentCmd) Undo() core.Result {
	result := undoPointContextSnapshot(c.Name(), c.pc, c.snapshot, c.hasSnapshot)
	if result.Signal != core.ToolDone {
		return result
	}
	return undo.BoundaryCompensationUndo(c.Name(), "restore point workspace artifacts and compensate the harness child process")
}
func (c *runAgentCmd) UndoMemento() (core.UndoMemento, error) {
	if !c.hasSnapshot {
		return core.UndoMemento{}, fmt.Errorf("%w: no point context snapshot recorded for %s", core.ErrUndoMementoMissing, c.Name())
	}
	return undo.BoundaryCompensationMemento(c.Name(), undo.BoundaryCompensationPayload{BoundaryCompensation: undo.BoundaryCompensation{
		Strategy:       "point_workspace_restore_and_child_process_compensation",
		Reason:         "runs the harness agent in the point workspace",
		Requires:       []string{"Workspace", "point_context_snapshot"},
		WorkspacePaths: []string{c.snapshot.point.PointDir},
		ArtifactPaths:  []string{c.snapshot.point.TracePath, c.snapshot.point.ResultPath},
		ChildRunID:     c.snapshot.point.PointID,
	}}, "restore point workspace artifacts and compensate the harness child process")
}

func (c *runAgentCmd) Execute() core.Result {
	pc := c.pc
	c.snapshot = snapshotPointContext(pc)
	c.hasSnapshot = true

	absTrace, _ := filepath.Abs(pc.TracePath)
	args := []string{
		"--directory", pc.PointDir,
		"--otel-log-file", absTrace,
	}

	if pc.ProfilePath == "" {
		err := fmt.Errorf("run_agent: profile path is required")
		return core.Result{CommandName: c.Name(), Signal: core.CommandError, Err: err, Output: err.Error()}
	}
	args = append(args, "--profile", pc.ProfilePath)

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
