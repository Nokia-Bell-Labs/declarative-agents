// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package catalog

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSpecPinProfileLoadsWithoutName pins R-PROF-001 as the loader enforces
// it: an agent-profile needs its machine and tools, not a name.
func TestSpecPinProfileLoadsWithoutName(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	path := writeToolImportFixture(t, root, filepath.Join("agents", "one", "profile.yaml"),
		"machine: machine.yaml\ntools: [tools.yaml]\n")

	profile, err := LoadProfile(path)

	require.NoError(t, err)
	require.Empty(t, profile.Name)
}

// TestSpecPinProfileRejectsBlueprintKeynames pins R-PROF-002 and R-PROF-003:
// the profile decoder is strict and has no expansion or blueprint keynames, so
// an agent-profile can neither expand a blueprint nor be one.
func TestSpecPinProfileRejectsBlueprintKeynames(t *testing.T) {
	t.Parallel()
	for keyname, extra := range map[string]string{
		"expand":  "expand:\n  - {fragment: blueprint.yaml, args: {agent: one}}\n",
		"unit":    "unit: one-blueprint\n",
		"params":  "params:\n  - {name: agent, type: string}\n",
		"profile": "profile: {machine: machine.yaml}\n",
	} {
		root := t.TempDir()
		path := writeToolImportFixture(t, root, filepath.Join("agents", "one", "profile.yaml"),
			"name: one\nmachine: machine.yaml\ntools: [tools.yaml]\n"+extra)

		_, err := LoadProfile(path)

		require.Error(t, err, keyname)
		require.ErrorContains(t, err, keyname, keyname)
	}
}
