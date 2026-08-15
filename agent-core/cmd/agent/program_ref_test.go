// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

func TestAugmentRollbackResourcesLoadsVerifiedOriginatingProgram(t *testing.T) {
	t.Parallel()
	profile, override := writeRollbackOriginProgram(t)
	runtime := runtimeConfig{Directory: t.TempDir()}
	targetConfig, err := runtimeConfigForProfile(profile, runtime)
	require.NoError(t, err)
	ref, err := buildProgramRef(targetConfig)
	require.NoError(t, err)

	checkpoint := &core.InMemoryCheckpoint{}
	require.NoError(t, checkpoint.Save(core.Position{
		Snapshot: core.AgentSnapshot{Program: ref},
	}, core.Execution{{CommandName: "origin_write"}}))
	current := runResources{
		Config: runtime,
		Definitions: []catalog.ToolDef{{
			Name: "checkpoint_rollback", Type: "builtin", Init: "checkpoint_rollback",
		}},
	}

	augmented, err := augmentRollbackResources(current, checkpoint)
	require.NoError(t, err)
	require.Equal(t, "true", definitionByName(t, augmented.Definitions, "origin_write").Binary,
		"profile-local declaration must override the config-dir declaration")
	require.Equal(t, "checkpoint_rollback",
		definitionByName(t, augmented.Definitions, "checkpoint_rollback").Name)

	require.NoError(t, os.WriteFile(override, []byte(originOverrideYAML+"# drift\n"), 0o600))
	_, err = augmentRollbackResources(current, checkpoint)
	require.ErrorContains(t, err, "target program")
	require.ErrorContains(t, err, "changed")
}

func TestAugmentRollbackResourcesRejectsLegacyCheckpointWithoutProgram(t *testing.T) {
	t.Parallel()
	checkpoint := &core.InMemoryCheckpoint{}
	require.NoError(t, checkpoint.Save(core.Position{}, nil))
	resources := runResources{
		Config: runtimeConfig{},
		Definitions: []catalog.ToolDef{{
			Name: "checkpoint_rollback", Type: "builtin", Init: "checkpoint_rollback",
		}},
	}

	_, err := augmentRollbackResources(resources, checkpoint)
	require.ErrorContains(t, err, "no declarative program reference")
}

func TestAugmentRollbackResourcesRejectsMissingReceiptBuilder(t *testing.T) {
	t.Parallel()
	profile, _ := writeRollbackOriginProgram(t)
	targetConfig, err := runtimeConfigForProfile(profile, runtimeConfig{})
	require.NoError(t, err)
	ref, err := buildProgramRef(targetConfig)
	require.NoError(t, err)
	checkpoint := &core.InMemoryCheckpoint{}
	require.NoError(t, checkpoint.Save(core.Position{
		Snapshot: core.AgentSnapshot{Program: ref},
	}, core.Execution{{CommandName: "missing_word", Receipt: "opaque"}}))

	_, err = augmentRollbackResources(rollbackFixtureResources(t.TempDir()), checkpoint)
	require.ErrorContains(t, err, `does not declare receipt-bearing command "missing_word"`)
}

const originOverrideYAML = `tools:
  - name: origin_write
    type: exec
    binary: "true"
    undo: {strategy: workspace_restore}
`

func writeRollbackOriginProgram(t *testing.T) (profilePath, overridePath string) {
	t.Helper()
	dir := t.TempDir()
	shared := filepath.Join(dir, "shared")
	require.NoError(t, os.MkdirAll(shared, 0o755))
	writeTestFile(t, filepath.Join(dir, "machine.yaml"), "name: origin\n")
	writeTestFile(t, filepath.Join(dir, "tools.yaml"), "tools: [origin_write]\n")
	writeTestFile(t, filepath.Join(shared, "origin.yaml"), `tools:
  - name: origin_write
    type: exec
    binary: "false"
    undo: {strategy: workspace_restore}
`)
	overridePath = filepath.Join(dir, "override.yaml")
	writeTestFile(t, overridePath, originOverrideYAML)
	profilePath = filepath.Join(dir, "profile.yaml")
	writeTestFile(t, profilePath, `name: origin
machine: machine.yaml
tools: [tools.yaml]
tool_config_dirs: [shared]
tool_declarations: [override.yaml]
`)
	return profilePath, overridePath
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

func definitionByName(t *testing.T, defs []catalog.ToolDef, name string) catalog.ToolDef {
	t.Helper()
	for _, def := range defs {
		if def.Name == name {
			return def
		}
	}
	t.Fatalf("definition %q not found", name)
	return catalog.ToolDef{}
}
