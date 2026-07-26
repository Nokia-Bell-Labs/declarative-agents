// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func TestResolveRunIDFreshRunsDifferAndResumeRetainsID(t *testing.T) {
	first, err := resolveRunID(runtimeConfig{})
	require.NoError(t, err)
	second, err := resolveRunID(runtimeConfig{})
	require.NoError(t, err)

	require.NotEmpty(t, first)
	require.NotEqual(t, first, second)

	resumed, err := resolveRunID(runtimeConfig{ResumeCheckpoint: first})
	require.NoError(t, err)
	require.Equal(t, first, resumed)
}

func TestResolveRunIDRejectsUnsupportedLatestAlias(t *testing.T) {
	_, err := resolveRunID(runtimeConfig{ResumeCheckpoint: "latest"})

	require.ErrorContains(t, err, "--resume-checkpoint")
	require.ErrorContains(t, err, "provide an explicit run id")
}

func TestRunIDIsSharedByCheckpointAndLoopWithoutChangingAgentName(t *testing.T) {
	originalOpen := openDoltCheckpoint
	t.Cleanup(func() { openDoltCheckpoint = originalOpen })

	const runID = "run-shared"
	var checkpointRunID string
	checkpoint := &closingCheckpoint{}
	openDoltCheckpoint = func(_, id string, _ func(core.State) bool) (closeableCheckpoint, error) {
		checkpointRunID = id
		return checkpoint, nil
	}

	opened, err := resolveCheckpoint(
		runtimeConfig{DoltDSN: "test-dsn"},
		core.MachineSpec{},
		runID,
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, opened.close()) })

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
