// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/service"
)

func TestPreparedRunCloseReapsServiceChildren(t *testing.T) {
	state := service.NewState()
	script := filepath.Join(t.TempDir(), "serve-child")
	require.NoError(t, os.WriteFile(
		script, []byte("#!/bin/sh\nsleep 30\n"), 0o755,
	))
	_, err := state.Start(service.StartSpec{
		Name: "leaked", Binary: script, Profile: "profile",
	})
	require.NoError(t, err)
	require.Len(t, state.Running(), 1)

	prepared := preparedRun{State: &agentState{
		reapServices: func() { state.Reap() },
	}}
	require.NoError(t, prepared.Close())
	require.Empty(t, state.Running())
	require.NoError(t, prepared.Close(), "cleanup stays idempotent")
}
