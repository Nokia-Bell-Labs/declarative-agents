// Copyright (c) 2026 Nokia. All rights reserved.

package pipeline

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/planning/extract"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/planning/plan"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/execute"
)

func TestExecuteTaskCommandBehavior(t *testing.T) {
	t.Parallel()
	runErr := errors.New("child launch failed")
	tests := []struct {
		name       string
		state      *State
		result     *execute.Result
		runErr     error
		wantSignal core.Signal
		wantOutput string
	}{
		{name: "missing task and plan", state: &State{}, wantSignal: core.CommandError, wantOutput: "no current task or plan"},
		{name: "success", state: executableTaskState(), result: &execute.Result{Stdout: "done", Duration: 2 * time.Second}, wantSignal: SigExecutionDone, wantOutput: "done"},
		{name: "malformed output remains opaque", state: executableTaskState(), result: &execute.Result{Stdout: "\x00not-json"}, wantSignal: SigExecutionDone, wantOutput: "\x00not-json"},
		{name: "child nonzero", state: executableTaskState(), result: &execute.Result{ExitCode: 7, Stdout: "out", Stderr: "err"}, wantSignal: SigExecutionFailed, wantOutput: "exit 7"},
		{name: "timeout", state: executableTaskState(), result: &execute.Result{ExitCode: -1, TimedOut: true, Stderr: "deadline"}, wantSignal: SigExecutionFailed, wantOutput: "exit -1"},
		{name: "runner error", state: executableTaskState(), runErr: runErr, wantSignal: core.CommandError, wantOutput: "child launch failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			calls := 0
			cmd := &executeTaskCmd{ps: tt.state, run: func(*State) (*execute.Result, error) {
				calls++
				return tt.result, tt.runErr
			}}
			result := cmd.Execute()
			assert.Equal(t, tt.wantSignal, result.Signal)
			assert.Contains(t, result.Output, tt.wantOutput)
			assert.Equal(t, "execute_task", result.CommandName)
			if tt.state.CurrentTask == nil || tt.state.CurrentPlan == nil {
				assert.Zero(t, calls)
			} else {
				assert.Equal(t, 1, calls)
			}
		})
	}
}

func TestExecuteTaskBuilderAndUndoContract(t *testing.T) {
	t.Parallel()
	state := executableTaskState()
	runner := func(*State) (*execute.Result, error) {
		return &execute.Result{Stdout: "built"}, nil
	}
	command := (&ExecuteTaskBuilder{PS: state, run: runner}).Build(core.Result{})
	require.Equal(t, "execute_task", command.Name())
	require.Equal(t, SigExecutionDone, command.Execute().Signal)

	undoResult := command.Undo(core.Result{})
	require.Equal(t, core.CommandError, undoResult.Signal)
	require.ErrorContains(t, undoResult.Err, "requires child agent history or workspace compensation")
}

func executableTaskState() *State {
	return &State{
		CurrentTask: &extract.Task{ID: "task-1"},
		CurrentPlan: &plan.ImplementationPlan{},
	}
}
