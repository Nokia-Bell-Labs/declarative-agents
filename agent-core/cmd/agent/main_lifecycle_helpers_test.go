// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"context"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/lifecycle"
	"github.com/stretchr/testify/require"
	"path/filepath"
	"testing"
)

type exitMachineCase struct {
	machinePath   string
	launch        string
	secondLaunch  string
	monitorLaunch string
	monitorStop   string
	docsStop      string
	await         string
	terminal      string
	shutdown      *deferredShutdown
}

type staticSignalBuilder struct {
	name      string
	signal    core.Signal
	output    string
	afterExit core.Signal
}

type staticSignalCmd struct {
	name   string
	signal core.Signal
	output string
}

func runExitMachine(t *testing.T, tc exitMachineCase) core.RunResult {
	t.Helper()
	machinePath := tc.machinePath
	if !filepath.IsAbs(machinePath) {
		machinePath = filepath.Join(repoRootFromTest(t), machinePath)
	}
	machine, err := core.LoadMachineSpec(machinePath)
	require.NoError(t, err)
	reg := core.NewRegistry()
	launchStopSignal := core.Signal("")
	if tc.secondLaunch != "" {
		launchStopSignal = "ServerStopped"
	}
	registerStaticSignal(reg, tc.launch, "ServerLaunched", "{}", launchStopSignal)
	if tc.secondLaunch != "" {
		registerStaticSignal(reg, tc.secondLaunch, "ServerLaunched", "{}", "")
	}
	if tc.monitorLaunch != "" {
		registerStaticSignal(reg, tc.monitorLaunch, "ServerLaunched", "{}", "")
	}
	if tc.monitorStop != "" {
		registerStaticSignal(reg, tc.monitorStop, "ServerStopped", "{}", "")
	}
	if tc.docsStop != "" {
		registerStaticSignal(reg, tc.docsStop, "ServerStopped", "{}", "")
	}
	registerStaticSignal(reg, tc.await, "ExitRequested", exitEventOutput(), "")
	reg.Register(core.ToolSpec{Name: "exit_agent"}, lifecycle.ExitBuilder{
		Config:   lifecycle.ExitConfig{Status: "success"},
		Shutdown: tc.shutdown.Request,
	})
	result, err := core.Loop(core.LoopParams{
		MachineSpec: &machine, Registry: reg, Trace: tracing.NoopTracer{},
	}, context.Background())
	require.NoError(t, err)
	return result
}

func registerStaticSignal(reg *core.Registry, name string, signal core.Signal, output string, afterExit core.Signal) {
	reg.Register(core.ToolSpec{Name: name}, staticSignalBuilder{
		name: name, signal: signal, output: output, afterExit: afterExit,
	})
}

func isTeardownSignal(signal core.Signal) bool {
	return signal == core.Signal("AgentExited") || signal == core.Signal("ServerStopped")
}

func exitEventOutput() string {
	return `{"payload":{"reason":"operator requested shutdown","status":"success"}}`
}

func requireExitEvent(t *testing.T, result core.RunResult) {
	t.Helper()
	require.NotEqual(t, core.StatusCancelled, result.Status)
	for _, event := range result.Events {
		if event.CommandName == "exit_agent" {
			require.Equal(t, core.Signal("AgentExited"), event.Signal)
			return
		}
	}
	require.Fail(t, "exit_agent event not recorded")
}
