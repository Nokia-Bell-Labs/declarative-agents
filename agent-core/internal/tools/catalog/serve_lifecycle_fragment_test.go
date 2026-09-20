// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package catalog

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/corepath"
)

// The shipped serve-lifecycle fragment (srd057 R1). A serving agent's four
// lifecycle words are one instantiation, and the names they arrive under are
// arguments, so the machine that references them needs no change. The install
// root is process-scoped, so these tests do not run in parallel.

const serveLifecycleFragment = "/opt/agent-core/tools/units/serve-lifecycle-declarations-fragment.yaml"

func serveLifecycleArgs(extra string) string {
	return `unit: probe
expand:
- fragment: ` + serveLifecycleFragment + `
  args:
    agent: probe
    launch_requests: launch_probe_requests
    launch_control: launch_probe_control
    await_control: await_probe_control
    stop_requests: stop_probe_requests
    requests_rest_ref: probe_requests
    control_rest_ref: probe_control
    requests_noun: probe
    requests_purpose: for probing
` + extra + `tools:
`
}

// installedAgentCore points the library root at the tree this test runs in, so
// the assertions cover the shipped fragment rather than a fixture copy of it.
func installedAgentCore(t *testing.T) {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	require.NoError(t, err)
	corepath.SetInstallRoot(root)
	t.Cleanup(func() { corepath.SetInstallRoot("") })
}

func TestServeLifecycleFragmentProducesTheWordsItsArgumentsName(t *testing.T) {
	installedAgentCore(t)
	top := writeToolImportFixture(t, t.TempDir(), "agents/probe/declarations.yaml", serveLifecycleArgs(""))

	defs, err := newToolImportResolver(nil).loadRoots([]string{top})

	require.NoError(t, err)
	require.ElementsMatch(t, []string{
		"launch_probe_requests", "launch_probe_control", "await_probe_control", "stop_probe_requests",
	}, toolNames(defs), "each word arrives under the name its argument gave it")
}

func TestServeLifecycleFragmentWiresEachWordToItsRestDefinition(t *testing.T) {
	installedAgentCore(t)
	top := writeToolImportFixture(t, t.TempDir(), "agents/probe/declarations.yaml", serveLifecycleArgs(""))

	defs, err := newToolImportResolver(nil).loadRoots([]string{top})

	require.NoError(t, err)
	byName := map[string]ToolDef{}
	for _, def := range defs {
		byName[def.Name] = def
	}
	require.Equal(t, "rest_server_launch", byName["launch_probe_requests"].Init)
	require.Equal(t, "rest_await_event", byName["await_probe_control"].Init)
	require.Equal(t, "rest_server_stop", byName["stop_probe_requests"].Init)
	require.Equal(t, "probe_requests", byName["launch_probe_requests"].Config["rest_ref"],
		"the request server launches the definition its argument named")
	require.Equal(t, "probe_control", byName["launch_probe_control"].Config["rest_ref"],
		"the control server launches its own definition, not the request one")
}

func TestServeLifecycleFragmentRequiresEveryWordName(t *testing.T) {
	installedAgentCore(t)
	missing := `unit: probe
expand:
- fragment: ` + serveLifecycleFragment + `
  args: {agent: probe, launch_requests: launch_probe_requests}
tools:
`
	top := writeToolImportFixture(t, t.TempDir(), "agents/probe/declarations.yaml", missing)

	_, err := newToolImportResolver(nil).loadRoots([]string{top})

	require.Error(t, err)
	require.Contains(t, err.Error(), "launch_control", "the error names the parameter that was not supplied")
}
