// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package definition

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/corepath"
)

// The shipped monitor server fragment (srd057 R1). Every agent serves the same
// eight monitor views and differs only in where it binds, so the server is one
// instantiation. The install root is process-scoped, so these tests do not run
// in parallel.

const monitorServerFragment = "/opt/agent-core/tools/rest/units/monitor-server-fragment.yaml"

// installedAgentCore points the library root at the tree this test runs in, so
// the assertions cover the shipped fragment rather than a fixture copy of it.
func installedAgentCore(t *testing.T) {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	require.NoError(t, err)
	corepath.SetInstallRoot(root)
	t.Cleanup(func() { corepath.SetInstallRoot("") })
}

func monitorInstantiation(t *testing.T) string {
	t.Helper()
	return writeImportFixture(t, t.TempDir(), "monitor-rest.yaml", `unit: probe-monitor-rest
expand:
- fragment: `+monitorServerFragment+`
  args: {address: "127.0.0.1:19999", limits_ref: probe_monitor, queue_name: probe_monitor}
rest:
  version: v1
  limits:
    probe_monitor: {max_request_bytes: 4096}
`)
}

func TestMonitorServerFragmentProducesTheEightViews(t *testing.T) {
	installedAgentCore(t)

	def, err := LoadDefinitionClosure([]string{monitorInstantiation(t)}, nil)

	require.NoError(t, err)
	server, ok := def.Servers["monitor"]
	require.True(t, ok, "the fragment produces a server named monitor")
	views := map[string]string{}
	for name, endpoint := range server.Endpoints {
		views[name] = endpoint.MonitorView
	}
	require.Equal(t, map[string]string{
		"machine_spec":      "machine_spec",
		"declared_machines": "declared_machines",
		"declared_tools":    "declared_tools",
		"current_state":     "current_state",
		"tools":             "tools",
		"metrics":           "metrics",
		"recent_events":     "events",
		"event_stream":      "events",
	}, views, "the monitor surface is fixed; srd057 pins it for the shared panels")
}

func TestMonitorServerFragmentBindsWhereItsArgumentsSay(t *testing.T) {
	installedAgentCore(t)

	def, err := LoadDefinitionClosure([]string{monitorInstantiation(t)}, nil)

	require.NoError(t, err)
	server := def.Servers["monitor"]
	require.Equal(t, "127.0.0.1:19999", server.Address)
	require.Equal(t, "probe_monitor", server.LimitsRef)
	require.True(t, server.LifecycleExit.Disabled, "a monitor server never carries the lifecycle exit")
}

func TestMonitorServerFragmentRequiresItsAddress(t *testing.T) {
	installedAgentCore(t)
	top := writeImportFixture(t, t.TempDir(), "monitor-rest.yaml", `unit: probe-monitor-rest
expand:
- fragment: `+monitorServerFragment+`
  args: {limits_ref: probe_monitor, queue_name: probe_monitor}
rest: {}
`)

	_, err := LoadDefinitionClosure([]string{top}, nil)

	require.Error(t, err)
	require.Contains(t, err.Error(), "address", "the error names the parameter that was not supplied")
}
