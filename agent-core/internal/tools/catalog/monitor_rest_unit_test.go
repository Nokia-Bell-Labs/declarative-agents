// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package catalog

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The shipped monitor REST pair (srd057 R1). The monitor server fragment names
// its server `monitor` in every agent, so the pair is the same everywhere and
// arrives by import beside the serve-lifecycle instantiation.

const monitorRestUnit = "/opt/agent-core/tools/units/monitor-rest-declarations.yaml"

func TestMonitorRestUnitSuppliesThePairBesideTheLifecycle(t *testing.T) {
	installedAgentCore(t)
	withPair := serveLifecycleArgs("")
	withPair = "imports:\n- " + monitorRestUnit + "\n" + withPair
	top := writeToolImportFixture(t, t.TempDir(), "agents/probe/declarations.yaml", withPair)

	defs, err := newToolImportResolver(nil).loadRoots([]string{top})

	require.NoError(t, err)
	require.ElementsMatch(t, []string{
		"launch_probe_requests", "launch_probe_control", "await_probe_control", "stop_probe_requests",
		"launch_monitor_rest", "stop_monitor_rest",
	}, toolNames(defs), "the import and the instantiation compose without colliding")
}

func TestMonitorRestUnitWiresBothWordsToTheMonitorServerWithTypedContracts(t *testing.T) {
	installedAgentCore(t)
	top := writeToolImportFixture(t, t.TempDir(), "agents/probe/declarations.yaml",
		"unit: probe\nimports:\n- "+monitorRestUnit+"\ntools:\n")

	defs, err := newToolImportResolver(nil).loadRoots([]string{top})

	require.NoError(t, err)
	byName := map[string]ToolDef{}
	for _, def := range defs {
		byName[def.Name] = def
	}
	require.Equal(t, "rest_server_launch", byName["launch_monitor_rest"].Init)
	require.Equal(t, "rest_server_stop", byName["stop_monitor_rest"].Init)
	for _, name := range []string{"launch_monitor_rest", "stop_monitor_rest"} {
		def := byName[name]
		require.Equal(t, "monitor", def.Config["rest_ref"],
			"%s names the server the monitor server fragment always produces", name)
		require.NotNil(t, def.Signature, "%s carries a declared contract", name)
		require.Equal(t, "builtin-types-core.NoParameters", def.Signature.Input, name)
	}
	require.Equal(t, "builtin-types-server.ServerLaunch", byName["launch_monitor_rest"].Signature.Output)
	require.Equal(t, "builtin-types-server.ServerStop", byName["stop_monitor_rest"].Signature.Output)
}
