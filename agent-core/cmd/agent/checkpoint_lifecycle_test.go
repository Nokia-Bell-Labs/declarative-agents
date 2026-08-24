// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	rtcheckpoint "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/checkpoint"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

type closingCheckpoint struct {
	name      string
	saveErr   error
	closeErr  error
	closeHook func()
	events    *[]string
	closes    int
}

func (c *closingCheckpoint) Save(core.Position, core.Execution) error { return c.saveErr }

func (c *closingCheckpoint) Load() (core.Position, core.Execution, error) {
	return core.Position{}, core.Execution{}, core.ErrNoCheckpoint
}

func (c *closingCheckpoint) Close() error {
	c.closes++
	if c.closeHook != nil {
		c.closeHook()
	}
	if c.events != nil {
		*c.events = append(*c.events, "close "+c.name)
	}
	return c.closeErr
}

func TestCheckpointResourcesCloseDistinctHandlesOnce(t *testing.T) {
	var events []string
	loopErr := errors.New("loop close")
	targetErr := errors.New("target close")
	loop := &closingCheckpoint{name: "loop", closeErr: loopErr, events: &events}
	target := &closingCheckpoint{name: "target", closeErr: targetErr, events: &events}
	resources := checkpointResources{}
	resources.Add(openedCheckpoint{Checkpoint: loop, CloseFunc: loop.Close, Label: "loop checkpoint"})
	resources.Add(openedCheckpoint{Checkpoint: target, CloseFunc: target.Close, Label: "target checkpoint"})

	err := resources.Close()
	secondErr := resources.Close()

	require.ErrorIs(t, err, loopErr)
	require.ErrorIs(t, err, targetErr)
	require.NoError(t, secondErr)
	require.Equal(t, []string{"close target", "close loop"}, events)
	require.Equal(t, 1, loop.closes)
	require.Equal(t, 1, target.closes)
}

func TestBuildPreparedRunTargetOpenFailureClosesLoopAndPreservesErrors(t *testing.T) {
	originalOpen := rtcheckpoint.OpenDolt
	t.Cleanup(func() { rtcheckpoint.OpenDolt = originalOpen })

	var events []string
	loopCloseErr := errors.New("loop close")
	targetOpenErr := errors.New("target open")
	loop := &closingCheckpoint{name: "loop", closeErr: loopCloseErr, events: &events}
	calls := 0
	rtcheckpoint.OpenDolt = func(_, _ string, _ func(core.State) bool) (closeableCheckpoint, error) {
		calls++
		if calls == 1 {
			return loop, nil
		}
		return nil, targetOpenErr
	}

	resources := lifecycleRunResources(t, &events)
	_, err := buildPreparedRun(&cobra.Command{}, resources)

	require.ErrorIs(t, err, targetOpenErr)
	require.ErrorIs(t, err, loopCloseErr)
	require.Equal(t, []string{"close loop", "telemetry"}, events)
	require.Equal(t, 1, loop.closes)
}

func TestBuildPreparedRunRegistrationFailureClosesBothBeforeTelemetry(t *testing.T) {
	originalOpen := rtcheckpoint.OpenDolt
	t.Cleanup(func() { rtcheckpoint.OpenDolt = originalOpen })

	var events []string
	loop := &closingCheckpoint{name: "loop", events: &events}
	targetCloseErr := errors.New("target close")
	target := &closingCheckpoint{name: "target", closeErr: targetCloseErr, events: &events}
	opened := []closeableCheckpoint{loop, target}
	rtcheckpoint.OpenDolt = func(_, _ string, _ func(core.State) bool) (closeableCheckpoint, error) {
		checkpoint := opened[0]
		opened = opened[1:]
		return checkpoint, nil
	}

	resources := lifecycleRunResources(t, &events)
	resources.Definitions = append(resources.Definitions, catalog.ToolDef{
		Name: "bad", Type: "builtin", Init: "missing",
	})
	_, err := buildPreparedRun(&cobra.Command{}, resources)

	require.ErrorContains(t, err, `unknown init "missing"`)
	require.ErrorIs(t, err, targetCloseErr)
	require.Equal(t, []string{"close target", "close loop", "telemetry"}, events)
	require.Equal(t, 1, loop.closes)
	require.Equal(t, 1, target.closes)
}

func TestPreparedRunCloseCancelsBeforeCheckpointAndTelemetry(t *testing.T) {
	var events []string
	ctx, cancel := context.WithCancel(context.Background())
	checkpoint := &closingCheckpoint{name: "loop", events: &events}
	checkpoint.closeHook = func() {
		require.ErrorIs(t, ctx.Err(), context.Canceled)
	}
	prepared := preparedRun{
		Ctx: ctx, Cancel: cancel,
		checkpoints: checkpointResources{opened: []openedCheckpoint{{
			Checkpoint: checkpoint, CloseFunc: checkpoint.Close, Label: "loop checkpoint",
		}}},
		shutdownTelemetry: func() { events = append(events, "telemetry") },
	}

	require.NoError(t, prepared.Close())
	require.NoError(t, prepared.Close())

	require.Equal(t, []string{"close loop", "telemetry"}, events)
	require.Equal(t, 1, checkpoint.closes)
}

func TestRunPreparedClosesCheckpointOnTerminalCancellationAndSuspension(t *testing.T) {
	originalExitCode := runExitCode
	t.Cleanup(func() { runExitCode = originalExitCode })

	tests := []struct {
		name   string
		params core.LoopParams
		cancel bool
	}{
		{name: "terminal success", params: terminalLoopParams()},
		{name: "cancellation", params: terminalLoopParams(), cancel: true},
		{name: "suspension", params: suspensionLoopParams()},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			checkpoint := &closingCheckpoint{name: "loop"}
			ctx, cancel := context.WithCancel(context.Background())
			if tc.cancel {
				cancel()
			}
			tc.params.Checkpoint = checkpoint
			prepared := preparedRun{
				Config: runtimeConfig{}, Params: tc.params, State: &agentState{},
				Ctx: ctx, Cancel: cancel, Shutdown: newDeferredShutdown(cancel),
				checkpoints: checkpointResources{opened: []openedCheckpoint{{
					Checkpoint: checkpoint, CloseFunc: checkpoint.Close, Label: "loop checkpoint",
				}}},
			}

			require.NoError(t, runPrepared(prepared))
			require.Equal(t, 1, checkpoint.closes)
		})
	}
}

func TestRunPreparedCheckpointFailuresReturnRunError(t *testing.T) {
	originalExitCode := runExitCode
	t.Cleanup(func() { runExitCode = originalExitCode })

	tests := []struct {
		name       string
		configure  func(*core.LoopParams, *closingCheckpoint)
		wantTyped  error
		wantDetail string
	}{
		{
			name: "Save",
			configure: func(_ *core.LoopParams, checkpoint *closingCheckpoint) {
				checkpoint.saveErr = errors.New("checkpoint database unavailable")
			},
			wantTyped:  core.ErrCheckpointSaveFailed,
			wantDetail: "checkpoint database unavailable",
		},
		{
			name: "conversation snapshot",
			configure: func(params *core.LoopParams, _ *closingCheckpoint) {
				params.Hooks.SnapshotConversation = func() (json.RawMessage, error) {
					return nil, errors.New("conversation encoder unavailable")
				}
			},
			wantTyped:  core.ErrConversationSnapshotFailed,
			wantDetail: "conversation encoder unavailable",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			checkpoint := &closingCheckpoint{name: "loop"}
			params := terminalLoopParams()
			params.Checkpoint = checkpoint
			tc.configure(&params, checkpoint)
			_, cancel := context.WithCancel(context.Background())
			prepared := preparedRun{
				Config: runtimeConfig{}, Params: params, State: &agentState{},
				Ctx: context.Background(), Cancel: cancel, Shutdown: newDeferredShutdown(cancel),
				checkpoints: checkpointResources{opened: []openedCheckpoint{{
					Checkpoint: checkpoint, CloseFunc: checkpoint.Close, Label: "loop checkpoint",
				}}},
			}
			runExitCode = ExitSucceeded

			err := runPrepared(prepared)

			require.ErrorIs(t, err, tc.wantTyped)
			require.ErrorContains(t, err, tc.wantDetail)
			require.Equal(t, ExitSucceeded, runExitCode, "run errors are handled by main as ExitRunError, not status-mapped")
			require.Equal(t, 1, checkpoint.closes)
		})
	}
}

func TestRunPreparedMachineFailureUsesMachineExitCode(t *testing.T) {
	originalExitCode := runExitCode
	t.Cleanup(func() { runExitCode = originalExitCode })
	params := terminalLoopParams()
	params.Table = core.TransitionTable{
		{State: "Start", Signal: core.Seed}: {NextState: "Rejected"},
	}
	params.IsTerminal = func(state core.State) bool { return state == "Rejected" }
	_, cancel := context.WithCancel(context.Background())
	runExitCode = ExitSucceeded

	err := runPrepared(preparedRun{
		Config: runtimeConfig{}, Params: params, State: &agentState{},
		Ctx: context.Background(), Cancel: cancel, Shutdown: newDeferredShutdown(cancel),
	})

	require.NoError(t, err)
	require.Equal(t, ExitMachineFailed, runExitCode)
}

func TestRunOrResumePreservesCheckpointFailureResult(t *testing.T) {
	checkpoint := &closingCheckpoint{saveErr: errors.New("checkpoint database unavailable")}
	params := terminalLoopParams()
	params.Checkpoint = checkpoint

	result, err := runOrResume(runtimeConfig{}, resumeDeps{
		Params: params,
		State:  &agentState{},
		Ctx:    context.Background(),
	})

	require.ErrorIs(t, err, core.ErrCheckpointSaveFailed)
	require.Equal(t, core.StatusFailed, result.Status)
	require.Equal(t, core.State("Finished"), result.FinalState)
	require.Zero(t, result.Iterations)
	require.ErrorIs(t, result.LastError, core.ErrCheckpointSaveFailed)
	require.ErrorContains(t, result.LastError, "checkpoint database unavailable")
}

func lifecycleRunResources(t *testing.T, events *[]string) runResources {
	t.Helper()
	dir := t.TempDir()
	request := filepath.Join(dir, "request.yaml")
	require.NoError(t, os.WriteFile(request, []byte("checkpoint: target-run\n"), 0o644))
	return runResources{
		Config: runtimeConfig{Checkpoint: rtcheckpoint.Config{DoltDSN: "test-dsn"}, Request: request},
		Tracer: tracing.NoopTracer{},
		Definitions: []catalog.ToolDef{{
			Name: "checkpoint_history", Type: "builtin", Init: "checkpoint_history",
			Config: map[string]interface{}{"checkpoint": "$request.checkpoint"},
		}},
		Machine: core.MachineSpec{},
		shutdownTelemetry: func() {
			*events = append(*events, "telemetry")
		},
	}
}

func terminalLoopParams() core.LoopParams {
	return core.LoopParams{
		InitialState: "Start",
		Registry:     core.NewRegistry(),
		Table: core.TransitionTable{
			{State: "Start", Signal: core.Seed}: {NextState: "Finished"},
		},
		IsTerminal: func(state core.State) bool { return state == "Finished" },
		Trace:      tracing.NoopTracer{},
		Budget:     core.Budget{MaxIterations: 2},
	}
}

func suspensionLoopParams() core.LoopParams {
	registry := core.NewRegistry()
	registry.Register(core.ToolSpec{Name: "suspend", Visibility: core.Internal}, staticSignalBuilder{
		name: "suspend", signal: core.AwaitApproval,
	})
	builder, _ := registry.Resolve("suspend")
	return core.LoopParams{
		InitialState: "Start",
		Registry:     registry,
		Table: core.TransitionTable{
			{State: "Start", Signal: core.Seed}: {
				NextState: "AwaitingApproval",
				Action:    func(result core.Result) core.Command { return builder.Build(result) },
			},
		},
		IsTerminal: func(core.State) bool { return false },
		Trace:      tracing.NoopTracer{},
		Budget:     core.Budget{MaxIterations: 2},
	}
}

func TestRunPreparedReturnsCloseErrorAfterSuccessfulRun(t *testing.T) {
	closeErr := errors.New("close failed")
	checkpoint := &closingCheckpoint{name: "loop", closeErr: closeErr}
	_, cancel := context.WithCancel(context.Background())
	prepared := preparedRun{
		Config: runtimeConfig{}, Params: terminalLoopParams(), State: &agentState{},
		Ctx: context.Background(), Cancel: cancel, Shutdown: newDeferredShutdown(cancel),
		checkpoints: checkpointResources{opened: []openedCheckpoint{{
			Checkpoint: checkpoint, CloseFunc: checkpoint.Close, Label: "loop checkpoint",
		}}},
	}
	prepared.Params.Checkpoint = checkpoint

	err := runPrepared(prepared)

	require.ErrorIs(t, err, closeErr)
	require.ErrorContains(t, err, "close loop checkpoint")
	require.Equal(t, 1, checkpoint.closes)
}

func TestRunPreparedSurfacesCloseErrorWithPrimaryLoopError(t *testing.T) {
	closeErr := errors.New("close failed")
	checkpoint := &closingCheckpoint{name: "loop", closeErr: closeErr}
	prepared := preparedRun{
		Config: runtimeConfig{},
		Params: core.LoopParams{
			MachineFile: filepath.Join(t.TempDir(), "missing-machine.yaml"),
			Registry:    core.NewRegistry(),
			Trace:       tracing.NoopTracer{},
		},
		State:    &agentState{},
		Ctx:      context.Background(),
		Shutdown: newDeferredShutdown(func() {}),
		checkpoints: checkpointResources{opened: []openedCheckpoint{{
			Checkpoint: checkpoint, CloseFunc: checkpoint.Close, Label: "loop checkpoint",
		}}},
	}

	err := runPrepared(prepared)

	require.ErrorContains(t, err, "loop:")
	require.ErrorIs(t, err, closeErr)
	require.Equal(t, 1, checkpoint.closes)
	require.Contains(t, fmt.Sprint(err), "close loop checkpoint")
}
