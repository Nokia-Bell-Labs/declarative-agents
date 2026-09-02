// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/telemetry"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func TestRuntimeSignalCancelsDispatchAndPublishesTrace(t *testing.T) {
	startedAt := time.Now()
	originalExitCode := runExitCode
	t.Cleanup(func() { runExitCode = originalExitCode })

	for _, tc := range []struct {
		name   string
		signal os.Signal
	}{
		{name: "interrupt", signal: os.Interrupt},
		{name: "terminate", signal: syscall.SIGTERM},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tracePath := filepath.Join(t.TempDir(), "trace.json")
			tracer, _, shutdownTelemetry, err := initRunTelemetry(runtimeConfig{
				Telemetry: telemetry.Config{LogFile: tracePath},
			})
			require.NoError(t, err)

			started := make(chan struct{})
			command := &signalBlockingCommand{started: started}
			baseCtx, cancel := context.WithCancel(context.Background())
			prepared := preparedRun{
				Config: runtimeConfig{},
				Params: core.LoopParams{
					InitialState: "Start",
					Registry:     core.NewRegistry(),
					Table: core.TransitionTable{
						{State: "Start", Signal: core.Seed}: {
							NextState: "Running",
							Action: func(core.Result) core.Command {
								return command
							},
						},
					},
					IsTerminal:     func(core.State) bool { return false },
					Trace:          tracer,
					Budget:         core.Budget{MaxIterations: 3},
					CommandTimeout: time.Minute,
				},
				State:             &agentState{},
				Ctx:               baseCtx,
				Cancel:            cancel,
				Shutdown:          newDeferredShutdown(cancel),
				shutdownTelemetry: shutdownTelemetry,
			}

			done := make(chan error, 1)
			go func() { done <- runPrepared(prepared) }()
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("blocking dispatch did not start")
			}

			process, err := os.FindProcess(os.Getpid())
			require.NoError(t, err)
			require.NoError(t, process.Signal(tc.signal))
			select {
			case err := <-done:
				require.NoError(t, err)
			case <-time.After(2 * time.Second):
				t.Fatal("signal did not stop the prepared run")
			}

			names, events := readSignalTrace(t, tracePath)
			require.True(t, names["agent.run"], "published trace lacks the root span")
			require.True(t, names["execute_tool block"], "published trace lacks the cancelled dispatch")
			require.True(t, events["init.registry_frozen"], "published trace lacks the registry stage")
		})
	}
	require.Less(t, time.Since(startedAt), 5*time.Second)
}

type signalBlockingCommand struct {
	started chan struct{}
	once    sync.Once
}

func (*signalBlockingCommand) Name() string { return "block" }

func (c *signalBlockingCommand) Execute() core.Result {
	return c.ExecuteContext(context.Background())
}

func (c *signalBlockingCommand) ExecuteContext(ctx context.Context) core.Result {
	c.once.Do(func() { close(c.started) })
	<-ctx.Done()
	return core.Result{
		Signal: core.CommandError, CommandName: c.Name(), Output: ctx.Err().Error(), Err: ctx.Err(),
	}
}

func (c *signalBlockingCommand) Undo(core.Result) core.Result {
	return core.NoopUndo(c.Name())
}

func readSignalTrace(t *testing.T, path string) (map[string]bool, map[string]bool) {
	t.Helper()
	file, err := os.Open(path)
	require.NoError(t, err)
	defer func() { require.NoError(t, file.Close()) }()

	names := make(map[string]bool)
	events := make(map[string]bool)
	decoder := json.NewDecoder(file)
	for {
		var span struct {
			Name   string
			Events []struct{ Name string }
		}
		if err := decoder.Decode(&span); err != nil {
			require.ErrorIs(t, err, io.EOF)
			break
		}
		names[span.Name] = true
		for _, event := range span.Events {
			events[event.Name] = true
		}
	}
	return names, events
}
