// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateConfigRejectsDeclaredButUnselectedMachineAction(t *testing.T) {
	restore := snapshotAgentFlags()
	t.Cleanup(func() { restoreAgentFlags(restore) })

	monitorDir := filepath.Dir(profilePathFromTest(t, "monitor/profile.yaml"))
	dir := t.TempDir()
	tools := filepath.Join(dir, "tools.yaml")
	require.NoError(t, os.WriteFile(tools, []byte(
		"tools:\n  - await_monitor_control\n  - stop_monitor_rest\n",
	), 0o644))
	profile := filepath.Join(dir, "profile.yaml")
	require.NoError(t, os.WriteFile(profile, []byte(fmt.Sprintf(
		"name: inactive-action\nmachine: %s\ntools:\n  - %s\ntool_declarations:\n  - %s\nrest_definitions:\n  - %s\n",
		filepath.Join(monitorDir, "machine.yaml"),
		tools,
		filepath.Join(monitorDir, "declarations.yaml"),
		filepath.Join(monitorDir, "rest.yaml"),
	)), 0o644))

	clearAgentFlags()
	flagProfile = profile
	flagValidateConfig = true

	_, err := captureStderr(t, func() error {
		return run(rootCmd, nil)
	})
	require.ErrorContains(t, err, "machine action validation")
	require.ErrorContains(t, err, `action "launch_monitor_rest" is not selected`)
}
