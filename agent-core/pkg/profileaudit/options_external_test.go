// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package profileaudit_test

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/pkg/profileaudit"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/pkg/spec"
	"github.com/stretchr/testify/require"
)

func TestInspectWithOptionsScopesCoreRootForExternalOwners(t *testing.T) {
	profile := externalProfile(t)
	first := externalCoreRoot(t, "11s")
	second := externalCoreRoot(t, "22s")
	spec.SetAgentCoreInstallRoot(first)
	t.Cleanup(func() { spec.SetAgentCoreInstallRoot("") })

	report, err := profileaudit.Inspect(profile)
	require.NoError(t, err)
	require.Len(t, report.Operations, 1)
	require.Equal(t, 11*time.Second, report.Operations[0].Duration)

	report, err = profileaudit.InspectWithOptions(profile, profileaudit.Options{CoreRoot: second})
	require.NoError(t, err)
	require.Len(t, report.Operations, 1)
	require.Equal(t, 22*time.Second, report.Operations[0].Duration)
	require.Equal(t, first, spec.AgentCoreInstallRoot(), "temporary mapping must be restored")

	const calls = 20
	var wg sync.WaitGroup
	errs := make(chan error, calls)
	for n := 0; n < calls; n++ {
		root, expected := first, 11*time.Second
		if n%2 == 1 {
			root, expected = second, 22*time.Second
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, inspectErr := profileaudit.InspectWithOptions(
				profile, profileaudit.Options{CoreRoot: root},
			)
			if inspectErr != nil {
				errs <- inspectErr
				return
			}
			if len(got.Operations) != 1 || got.Operations[0].Duration != expected {
				errs <- fmt.Errorf("root %s resolved %#v, want %s", root, got.Operations, expected)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	require.Equal(t, first, spec.AgentCoreInstallRoot(), "concurrent calls must not leak mappings")
}

func externalProfile(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeExternal(t, filepath.Join(root, "machine.yaml"), `
name: external-owner
initial_state: Idle
budget: {max_iterations: 2, command_timeout: 1m}
states: [Idle, {name: Done, run_status: succeeded}]
terminal_states: [Done]
signals: [Seed]
transitions:
  - {state: Idle, signal: Seed, next: Done, action: $tool}
`)
	writeExternal(t, filepath.Join(root, "tools.yaml"), "tools: [wait]\n")
	profile := filepath.Join(root, "profile.yaml")
	writeExternal(t, profile, `
name: external-owner
machine: machine.yaml
tools: [tools.yaml]
tool_config_dirs: [/opt/agent-core/tools/builtin]
`)
	return profile
}

func externalCoreRoot(t *testing.T, timeout string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "tools", "builtin")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	writeExternal(t, filepath.Join(dir, "wait.yaml"), `
tools:
  - name: wait
    type: builtin
    init: custom_await
    category: boundary
    visibility: external
    config: {timeout: `+timeout+`}
`)
	return root
}

func writeExternal(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}
