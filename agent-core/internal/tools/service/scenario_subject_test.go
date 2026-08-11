// Copyright (c) 2026 Nokia. All rights reserved.

package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func TestStartSubjectRejectsInvalidHealthBeforeSpawn(t *testing.T) {
	root := scenarioTree(t, map[string]map[string][]string{"alpha": {"happy": nil}})
	scenarioDir := filepath.Join(root, "alpha", testsDirName, "happy")
	require.NoError(t, os.WriteFile(
		filepath.Join(scenarioDir, scenarioManifestName),
		[]byte("subject_health_path: ftp://example.invalid/health\n"), 0o644,
	))
	state := NewState()
	session := NewScenarioSession(state)
	_, err := session.Seed([]string{root})
	require.NoError(t, err)
	_, ok, err := session.Next()
	require.NoError(t, err)
	require.True(t, ok)

	result := Builder{
		ToolName: "start_subject", Init: InitStartSubject,
		State: state, Session: session, Config: ToolConfig{Binary: os.Args[0]},
	}.Build(core.Result{}).Execute()
	require.Equal(t, core.CommandError, result.Signal)
	require.ErrorContains(t, result.Err, `scheme "ftp" is not allowed`)
	require.Empty(t, state.Running())
	name, baseURL := session.Subject()
	require.Empty(t, name)
	require.Empty(t, baseURL)
	require.Empty(t, session.Children())
}

func TestUndoStartedChildValidatesAndConsumesReceipt(t *testing.T) {
	state := NewState()
	output, err := state.Start(StartSpec{
		Name: "undo-child", Binary: os.Args[0], Profile: "profile",
		Env: []string{envChildMode + "=hang"},
	})
	require.NoError(t, err)
	require.NotEmpty(t, output)
	cmd := Builder{
		ToolName: "start_subject", Init: InitStartSubject, State: state,
		Config: ToolConfig{Grace: "1s"},
	}.Build(core.Result{})

	invalid := cmd.Undo(core.Result{Receipt: "{"})
	require.Equal(t, core.CommandError, invalid.Signal)
	require.ErrorContains(t, invalid.Err, "invalid child receipt")
	require.Len(t, state.Running(), 1)

	undone := cmd.Undo(core.Result{Receipt: `{"service":"undo-child"}`})
	require.Equal(t, SignalServiceStopped, undone.Signal)
	require.Empty(t, state.Running())
}
