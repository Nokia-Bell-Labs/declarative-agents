// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

func TestDefsDeclareRequestSourcesDetectsDeclaredSourceNotInitName(t *testing.T) {
	// A checkpoint tool with no declared source is not request-driven, even
	// though its init name matches a checkpoint op; identity no longer decides.
	require.False(t, defsDeclareRequestSources([]catalog.ToolDef{{
		Name: "checkpoint_history", Init: "checkpoint_history",
	}}))
	// A tool of any identity that declares a $request source is request-driven.
	require.True(t, defsDeclareRequestSources([]catalog.ToolDef{{
		Name: "checkpoint_rollback", Init: "checkpoint_rollback",
		Config: map[string]interface{}{"to_iteration": "$request.to_iteration"},
	}}))
}

func TestResolveRequestSourcesReplacesPresentAndDropsAbsent(t *testing.T) {
	step := 4
	defs := []catalog.ToolDef{{
		Name: "checkpoint_rollback",
		Config: map[string]interface{}{
			"checkpoint":   "$request.checkpoint",
			"to_iteration": "$request.to_iteration",
			"input":        "$from(prev).output",
		},
	}}

	require.NoError(t, resolveRequestSources(defs, lifecycleRequest{Checkpoint: "run-7", ToIteration: &step}))

	require.Equal(t, "run-7", defs[0].Config["checkpoint"])
	require.Equal(t, 4, defs[0].Config["to_iteration"])
	// Non-$request values are left untouched.
	require.Equal(t, "$from(prev).output", defs[0].Config["input"])
}

func TestResolveRequestSourcesDeletesUnsetSoDefaultsApply(t *testing.T) {
	defs := []catalog.ToolDef{{
		Name: "checkpoint_rollback",
		Config: map[string]interface{}{
			"checkpoint":   "$request.checkpoint",
			"to_iteration": "$request.to_iteration",
		},
	}}

	require.NoError(t, resolveRequestSources(defs, lifecycleRequest{}))

	_, hasCheckpoint := defs[0].Config["checkpoint"]
	_, hasIteration := defs[0].Config["to_iteration"]
	require.False(t, hasCheckpoint, "unset checkpoint must be removed so the tool selects latest")
	require.False(t, hasIteration, "unset to_iteration must be removed so rollback keeps its default")
}

func TestResolveRequestSourcesRejectsUnknownField(t *testing.T) {
	defs := []catalog.ToolDef{{
		Name:   "checkpoint_history",
		Config: map[string]interface{}{"checkpoint": "$request.mystery"},
	}}

	err := resolveRequestSources(defs, lifecycleRequest{})

	require.ErrorContains(t, err, "unknown request source $request.mystery")
}
