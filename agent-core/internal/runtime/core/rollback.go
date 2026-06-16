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
	Compensation        CompensationExecutor
}

// CompensationExecutor runs rollback compensation described by an undo memento.
type CompensationExecutor interface {
	Compensate(context.Context, UndoMemento) Result
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
	result := RollbackResult{TargetIteration: opts.TargetIteration}
	targetState, workspaceRef, err := resolveRollbackTarget(opts.History, opts.TargetIteration, opts.InitialWorkspaceRef)
	if err != nil {
		result.Partial = true
		return result, err
	}
	result.State = targetState
	result.WorkspaceRef = workspaceRef

	errs := rollbackUndoHistory(opts.History, opts.TargetIteration, &result)
	errs = append(errs, rollbackRestoreWorkspace(opts, workspaceRef)...)
	if len(errs) > 0 {
		result.Partial = true
		return result, errors.Join(errs...)
	}
	return result, nil
}

func rollbackUndoHistory(history History, targetIteration int, result *RollbackResult) []error {
	var errs []error
	for i := len(history) - 1; i >= 0; i-- {
		entry := history[i]
		if entry.Iteration <= targetIteration {
			continue
		}
		undo := rollbackUndoEntry(entry)
		result.Undone = append(result.Undone, undo)
		if undo.Error != "" {
			errs = append(errs, fmt.Errorf("undo %s at iteration %d: %s", undo.CommandName, undo.Iteration, undo.Error))
		}
	}
	return errs
}

func rollbackRestoreWorkspace(opts RollbackOptions, workspaceRef string) []error {
	if opts.Workspace == nil || workspaceRef == "" {
		return nil
	}
	ctx := rollbackContext(opts.Ctx)
	if err := opts.Workspace.Restore(ctx, workspaceRef); err != nil {
		rollbackTracer(opts.Tracer).Event("rollback.workspace_restore_failed",
			attribute.Int("target_iteration", opts.TargetIteration),
			attribute.String("workspace_ref", workspaceRef),
			attribute.String("error", err.Error()),
		)
		return []error{fmt.Errorf("restore workspace to %s: %w", workspaceRef, err)}
	}
	return nil
}

func rollbackContext(ctx context.Context) context.Context {
	if ctx != nil {
		return ctx
	}
	return context.Background()
}

func rollbackTracer(tr tracing.Tracer) tracing.Tracer {
	if tr != nil {
		return tr
	}
	return tracing.NoopTracer{}
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
