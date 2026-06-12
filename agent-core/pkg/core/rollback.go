// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"context"
	"errors"
	"fmt"

	"go.opentelemetry.io/otel/attribute"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/observability/tracing"
)

// RollbackOptions describes a rollback traversal over recorded history.
type RollbackOptions struct {
	History             History
	TargetIteration     int
	Workspace           Workspace
	InitialWorkspaceRef string
	Tracer              tracing.Tracer
	Ctx                 context.Context
}

// RollbackResult summarizes the attempted rollback. If Err is non-nil from
// Rollback, this result is still useful for partial-failure diagnostics.
type RollbackResult struct {
	TargetIteration int
	State           State
	WorkspaceRef    string
	Undone          []RollbackUndo
	Partial         bool
}

// RollbackUndo records the result of undoing one command.
type RollbackUndo struct {
	Iteration   int
	CommandName string
	Result      ResultDigest
	Error       string
}

// RollbackTo walks history backward to targetIteration. It always attempts all
// command undos and the workspace restore, then returns an aggregated error if
// any layer failed. This makes partial rollback explicit instead of hiding a
// mixed state behind the first failure.
func RollbackTo(opts RollbackOptions) (RollbackResult, error) {
	ctx := opts.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	tr := opts.Tracer
	if tr == nil {
		tr = tracing.NoopTracer{}
	}

	result := RollbackResult{TargetIteration: opts.TargetIteration}
	targetState, workspaceRef, err := resolveRollbackTarget(opts.History, opts.TargetIteration, opts.InitialWorkspaceRef)
	if err != nil {
		result.Partial = true
		return result, err
	}
	result.State = targetState
	result.WorkspaceRef = workspaceRef

	var errs []error
	for i := len(opts.History) - 1; i >= 0; i-- {
		entry := opts.History[i]
		if entry.Iteration <= opts.TargetIteration {
			continue
		}
		undo := rollbackUndoEntry(entry)
		result.Undone = append(result.Undone, undo)
		if undo.Error != "" {
			errs = append(errs, fmt.Errorf("undo %s at iteration %d: %s", undo.CommandName, undo.Iteration, undo.Error))
		}
	}

	if opts.Workspace != nil && workspaceRef != "" {
		if err := opts.Workspace.Restore(ctx, workspaceRef); err != nil {
			tr.Event("rollback.workspace_restore_failed",
				attribute.Int("target_iteration", opts.TargetIteration),
				attribute.String("workspace_ref", workspaceRef),
				attribute.String("error", err.Error()),
			)
			errs = append(errs, fmt.Errorf("restore workspace to %s: %w", workspaceRef, err))
		}
	}

	if len(errs) > 0 {
		result.Partial = true
		return result, errors.Join(errs...)
	}
	return result, nil
}

// Rollback is a convenience wrapper for callers that do not need to provide an
// initial workspace ref. Full rollback to iteration 0 will restore only command
// state unless RollbackTo is used with InitialWorkspaceRef.
func Rollback(history History, targetIteration int, workspace Workspace, tr tracing.Tracer) (RollbackResult, error) {
	return RollbackTo(RollbackOptions{
		History:         history,
		TargetIteration: targetIteration,
		Workspace:       workspace,
		Tracer:          tr,
	})
}

func resolveRollbackTarget(history History, targetIteration int, initialWorkspaceRef string) (State, string, error) {
	if targetIteration < 0 {
		return "", "", fmt.Errorf("rollback target iteration must be >= 0, got %d", targetIteration)
	}
	if len(history) == 0 {
		if targetIteration == 0 {
			return "", initialWorkspaceRef, nil
		}
		return "", "", fmt.Errorf("rollback target iteration %d not found in empty history", targetIteration)
	}
	if targetIteration == 0 {
		return history[0].FromState, initialWorkspaceRef, nil
	}
	for _, entry := range history {
		if entry.Iteration == targetIteration {
			return entry.ToState, entry.WorkspaceRef, nil
		}
	}
	return "", "", fmt.Errorf("rollback target iteration %d not found", targetIteration)
}

func rollbackUndoEntry(entry HistoryEntry) RollbackUndo {
	undo := RollbackUndo{
		Iteration:   entry.Iteration,
		CommandName: entry.CommandName,
	}
	if entry.Command == nil {
		undo.Error = "missing command"
		return undo
	}
	res := entry.Command.Undo()
	undo.Result = digestResult(res)
	if res.Err != nil {
		undo.Error = res.Err.Error()
		return undo
	}
	if res.Signal == CommandError {
		if res.Output != "" {
			undo.Error = res.Output
		} else {
			undo.Error = "command undo returned CommandError"
		}
	}
	return undo
}
