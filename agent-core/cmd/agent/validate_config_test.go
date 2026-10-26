// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The --validate-config boot smoke: a valid profile exits zero, and an
// invalid REST client, an implicit parse-retry policy, or a broken receipt
// contract exits non-zero before anything runs.

func TestValidateConfigValidProfileExitsZero(t *testing.T) {
	restore := snapshotAgentFlags()
	t.Cleanup(func() { restoreAgentFlags(restore) })

	for _, profile := range []string{"monitor/profile.yaml", "control-imports/profile.yaml"} {
		t.Run(profile, func(t *testing.T) {
			clearAgentFlags()
			flagProfile = profilePathFromTest(t, profile)
			flagValidateConfig = true

			stderr, err := captureStderr(t, func() error {
				return run(rootCmd, nil)
			})
			require.NoError(t, err)
			require.Contains(t, stderr, "config valid")
			// Validate mode must not enter the run loop or bind servers.
			require.NotContains(t, stderr, "\nterminal state:")
		})
	}
}

func TestValidateConfigInvalidRestExitsNonZero(t *testing.T) {
	restore := snapshotAgentFlags()
	t.Cleanup(func() { restoreAgentFlags(restore) })

	monitorDir := filepath.Dir(profilePathFromTest(t, "monitor/profile.yaml"))
	dir := t.TempDir()
	badRest := filepath.Join(dir, "rest.yaml")
	require.NoError(t, os.WriteFile(badRest,
		[]byte("rest:\n  version: v1\n  auth:\n    broken:\n      type: totally-unsupported\n"), 0o644))
	profile := filepath.Join(dir, "profile.yaml")
	require.NoError(t, os.WriteFile(profile, []byte(fmt.Sprintf(
		"name: badrest\nmachine: %s\ntools:\n  - %s\ntool_declarations:\n  - %s\nrest_definitions:\n  - %s\n",
		filepath.Join(monitorDir, "machine.yaml"),
		filepath.Join(monitorDir, "tools.yaml"),
		filepath.Join(monitorDir, "declarations.yaml"),
		badRest)), 0o644))

	clearAgentFlags()
	flagProfile = profile
	flagValidateConfig = true

	_, err := captureStderr(t, func() error {
		return run(rootCmd, nil)
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported type")
}

func TestValidateConfigRejectsImplicitParseRetryPolicy(t *testing.T) {
	restore := snapshotAgentFlags()
	t.Cleanup(func() { restoreAgentFlags(restore) })

	monitorDir := filepath.Dir(profilePathFromTest(t, "monitor/profile.yaml"))
	// The monitor machine is an instance of the monitor-service template
	// (srd054), so the budget text to corrupt lives in the template body.
	templateData, err := os.ReadFile(filepath.Join(
		repoRootFromTest(t), "tools", "machines", "monitor-service-machine-template.yaml"))
	require.NoError(t, err)
	patched := strings.Replace(
		string(templateData),
		"    max_iterations: 6",
		"    max_iterations: 6\n    max_consecutive_parse_errors: 2",
		1,
	)
	require.NotEqual(t, string(templateData), patched, "the template budget text moved")

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "template.yaml"), []byte(patched), 0o644))
	machine := filepath.Join(dir, "machine.yaml")
	require.NoError(t, os.WriteFile(machine, []byte(
		"name: monitor-rest-profile\nexpand:\n"+
			"  - fragment: template.yaml\n    args: {profile: monitor, machine: machine.yaml, tools: tools.yaml}\n"), 0o644))
	profile := filepath.Join(dir, "profile.yaml")
	require.NoError(t, os.WriteFile(profile, []byte(fmt.Sprintf(
		"name: implicit-parse-retry\nmachine: %s\ntools:\n  - %s\ntool_declarations:\n  - %s\nrest_definitions:\n  - %s\n",
		machine,
		filepath.Join(monitorDir, "tools.yaml"),
		filepath.Join(monitorDir, "declarations.yaml"),
		filepath.Join(monitorDir, "rest.yaml"),
	)), 0o644))

	clearAgentFlags()
	flagProfile = profile
	flagValidateConfig = true

	_, err = captureStderr(t, func() error {
		return run(rootCmd, nil)
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "parse retry wiring validation")
	require.Contains(t, err.Error(), `select "report_parse_error"`)
	require.Contains(t, err.Error(), "remove the parse retry budget")
}

func TestValidateConfigInvalidReceiptContractExitsNonZero(t *testing.T) {
	restore := snapshotAgentFlags()
	t.Cleanup(func() { restoreAgentFlags(restore) })

	monitorDir := filepath.Dir(profilePathFromTest(t, "monitor/profile.yaml"))
	// Corrupt one selected monitor tool to the inconsistent form GH-494
	// targets: reversible with a state-mutating effect but a noop undo.
	// --validate-config must reject it (srd025 R3.5; GH-494).
	realDecls, err := os.ReadFile(filepath.Join(monitorDir, "declarations.yaml"))
	require.NoError(t, err)
	corrupted := strings.Replace(string(realDecls),
		"      classification: reversible\n      undo: queue_event_restore",
		"      classification: reversible\n      undo: noop", 1)
	corrupted = strings.Replace(corrupted,
		"    strategy: queue_event_restore",
		"    strategy: noop", 1)
	require.NotEqual(t, string(realDecls), corrupted, "expected a reversible queue-restore tool to corrupt")

	dir := t.TempDir()
	badDecls := filepath.Join(dir, "declarations.yaml")
	require.NoError(t, os.WriteFile(badDecls, []byte(corrupted), 0o644))
	profile := filepath.Join(dir, "profile.yaml")
	require.NoError(t, os.WriteFile(profile, []byte(fmt.Sprintf(
		"name: badreceipt\nmachine: %s\ntools:\n  - %s\ntool_declarations:\n  - %s\nrest_definitions:\n  - %s\n",
		filepath.Join(monitorDir, "machine.yaml"),
		filepath.Join(monitorDir, "tools.yaml"),
		badDecls,
		filepath.Join(monitorDir, "rest.yaml"))), 0o644))

	clearAgentFlags()
	flagProfile = profile
	flagValidateConfig = true

	_, err = captureStderr(t, func() error {
		return run(rootCmd, nil)
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "receipt-contract validation failed")
	require.Contains(t, err.Error(), "no receipt-consuming undo")
}
