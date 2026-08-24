// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"testing"

	"github.com/stretchr/testify/require"

	monitorruntime "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/monitor/runtimeconfig"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	rtcheckpoint "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/checkpoint"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func TestResolveRunIDFreshRunsDifferAndResumeRetainsID(t *testing.T) {
	first, err := resolveRunID(runtimeConfig{})
	require.NoError(t, err)
	second, err := resolveRunID(runtimeConfig{})
	require.NoError(t, err)

	require.NotEmpty(t, first)
	require.NotEqual(t, first, second)

	resumed, err := resolveRunID(runtimeConfig{Checkpoint: rtcheckpoint.Config{ResumeCheckpoint: first}})
	require.NoError(t, err)
	require.Equal(t, first, resumed)
}

func TestResolveRunIDRejectsUnsupportedLatestAlias(t *testing.T) {
	_, err := resolveRunID(runtimeConfig{Checkpoint: rtcheckpoint.Config{ResumeCheckpoint: "latest"}})

	require.ErrorContains(t, err, "--resume-checkpoint")
	require.ErrorContains(t, err, "provide an explicit run id")
}

func TestRunIDIsSharedByCheckpointAndLoopWithoutChangingAgentName(t *testing.T) {
	originalOpen := rtcheckpoint.OpenDolt
	t.Cleanup(func() { rtcheckpoint.OpenDolt = originalOpen })

	const runID = "run-shared"
	var checkpointRunID string
	cp := &closingCheckpoint{}
	rtcheckpoint.OpenDolt = func(_, id string, _ func(core.State) bool) (closeableCheckpoint, error) {
		checkpointRunID = id
		return cp, nil
	}

	opened, err := rtcheckpoint.Config{DoltDSN: "test-dsn"}.Open(core.MachineSpec{}, runID)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, opened.Close()) })

	params := loopParams(runtimeConfig{}, loopParamDeps{
		Machine:  core.MachineSpec{},
		State:    &agentState{},
		Registry: core.NewRegistry(),
		Tracer:   tracing.NoopTracer{},
		RunID:    runID,
	})

	require.Equal(t, runID, checkpointRunID)
	require.Equal(t, runID, params.RunID)
	require.Equal(t, "agent", params.AgentName)
}

func TestMachineNameDrivesSpanAndMetricIdentity(t *testing.T) {
	t.Parallel()
	machine := core.MachineSpec{Name: "planner"}
	params := loopParams(runtimeConfig{}, loopParamDeps{
		Machine: machine, State: &agentState{}, Registry: core.NewRegistry(),
		Tracer: tracing.NoopTracer{},
	})
	require.Equal(t, "planner", params.AgentName)
	require.Equal(t, agentVersion, params.AgentVersion)

	monitorConfig, err := monitorruntime.CompileRecorderConfig(machine, nil, "run-planner")
	require.NoError(t, err)
	require.NotEmpty(t, monitorConfig.GlobalAttributes)
	require.Equal(t, "agent.name", monitorConfig.GlobalAttributes[0].Name)
	require.Equal(t, []string{"planner"}, monitorConfig.GlobalAttributes[0].AllowedValues)
	require.Equal(t, params.AgentName,
		monitorConfig.GlobalAttributes[0].AllowedValues[0],
		"span and metric identity must share one source")
}

func TestMachineAgentNameFallsBackForLegacySpec(t *testing.T) {
	t.Parallel()
	require.Equal(t, "agent", machineAgentName(core.MachineSpec{}))
}
