// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	internalload "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/load"
)

// TestStartupWiringFaultNamesTemplateAndInstance is srd054 R2.5: a machine
// that fails the startup wiring checks after instantiation is reported with the
// template that holds the fault beside the instance that selected it.
func TestStartupWiringFaultNamesTemplateAndInstance(t *testing.T) {
	root := repoRootFromTest(t)
	template, err := os.ReadFile(filepath.Join(root, "testdata", "integration", "units", "control-machine-template.yaml"))
	require.NoError(t, err)
	dropped := "    - {state: Awaiting, signal: ServerStopped, next: Failed}\n"
	require.Contains(t, string(template), dropped)
	directory := t.TempDir()
	brokenTemplate := filepath.Join(directory, "template.yaml")
	require.NoError(t, os.WriteFile(brokenTemplate, []byte(strings.Replace(string(template), dropped, "", 1)), 0o644))
	instance := filepath.Join(directory, "machine.yaml")
	require.NoError(t, os.WriteFile(instance, []byte("unit: broken\ninstantiate:\n"+
		"  - {fragment: template.yaml, args: {launch: launch_agent_control, await: await_agent_control}}\n"), 0o644))

	closure, err := internalload.LoadClosure(
		profilePathFromTest(t, "control-template/profile.yaml"), internalload.Options{MachineOverride: instance})
	require.NoError(t, err, "the machine is structurally valid; the fault is an unrouted emitted signal")

	_, err = loadValidatedRuntimeMachine(closure)

	require.ErrorContains(t, err, "has no transition for Awaiting/ServerStopped")
	require.ErrorContains(t, err, "instance of machine template "+brokenTemplate)
}
