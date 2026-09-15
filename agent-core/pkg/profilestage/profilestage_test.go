// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package profilestage_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/pkg/profilestage"
)

// writeDeclaration lays one declaration file down under root.
func writeDeclaration(t *testing.T, root, relative, body string) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	return path
}

// agentImportingAUnit is the shape every stager trips over: a declaration whose
// type unit sits beside its agent directory rather than inside it.
func agentImportingAUnit(t *testing.T) string {
	t.Helper()
	source := t.TempDir()
	writeDeclaration(t, source, "agents/collector/declarations.yaml",
		"unit: collector\nimports:\n- ../units/types-core.yaml\ntools: []\n")
	writeDeclaration(t, source, "agents/units/types-core.yaml",
		"unit: types-core\ntypes:\n- name: Text\n  schema:\n    type: string\n")
	return source
}

func TestStageCarriesASiblingImport(t *testing.T) {
	t.Parallel()
	source := agentImportingAUnit(t)
	destination := t.TempDir()

	require.NoError(t, profilestage.Stage(profilestage.Tree{
		Source:      filepath.Join(source, "agents", "collector"),
		Destination: filepath.Join(destination, "agents", "collector"),
	}))

	require.FileExists(t, filepath.Join(destination, "agents", "units", "types-core.yaml"),
		"the unit the staged declaration imports travels with it")
}

// TestPlantedViolationCopyingOnlyTheDirectory is the negative half: the copy
// every stager wrote by hand leaves the import dangling, which is the defect
// GH-2031 found five times and GH-2041 found twice more.
func TestPlantedViolationCopyingOnlyTheDirectory(t *testing.T) {
	t.Parallel()
	source := agentImportingAUnit(t)
	destination := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(destination, "agents", "collector"), 0o755))
	data, err := os.ReadFile(filepath.Join(source, "agents", "collector", "declarations.yaml"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(
		filepath.Join(destination, "agents", "collector", "declarations.yaml"), data, 0o644))

	require.NoFileExists(t, filepath.Join(destination, "agents", "units", "types-core.yaml"),
		"a directory copy stages the agent and drops what it imports")
}

// TestStageFollowsAReRootedImportToItsStagedPosition is the GH-2024 defect as a
// test. The applier projection drops the agents/ segment, so the unit belongs
// where the staged declaration's own relative path resolves, not where it sat.
func TestStageFollowsAReRootedImportToItsStagedPosition(t *testing.T) {
	t.Parallel()
	source := t.TempDir()
	writeDeclaration(t, source, "agents/applier/apply-declarations.yaml",
		"unit: applier\nimports:\n- ../units/types-core.yaml\ntools: []\n")
	writeDeclaration(t, source, "agents/units/types-core.yaml",
		"unit: types-core\ntypes: []\n")
	destination := t.TempDir()

	require.NoError(t, profilestage.Stage(profilestage.Tree{
		Source:      filepath.Join(source, "agents", "applier"),
		Destination: filepath.Join(destination, "applications", "catalog", "applier"),
	}))

	require.FileExists(t,
		filepath.Join(destination, "applications", "catalog", "units", "types-core.yaml"),
		"a re-rooted tree carries its imports to the position it re-rooted them to")
	require.NoFileExists(t, filepath.Join(destination, "agents", "units", "types-core.yaml"),
		"and not to the position they held at the source")
}

func TestStageResolvesImportsTransitively(t *testing.T) {
	t.Parallel()
	source := t.TempDir()
	writeDeclaration(t, source, "agents/chatbot/declarations.yaml",
		"unit: chatbot\nimports:\n- ../units/types-chatbot.yaml\ntools: []\n")
	writeDeclaration(t, source, "agents/units/types-chatbot.yaml",
		"unit: types-chatbot\nimports:\n- ../../shared/types-shared.yaml\ntypes: []\n")
	writeDeclaration(t, source, "shared/types-shared.yaml", "unit: types-shared\ntypes: []\n")
	destination := t.TempDir()

	require.NoError(t, profilestage.Stage(profilestage.Tree{
		Source:      filepath.Join(source, "agents", "chatbot"),
		Destination: filepath.Join(destination, "agents", "chatbot"),
	}))

	require.FileExists(t, filepath.Join(destination, "shared", "types-shared.yaml"),
		"a unit that imports another unit brings that one too")
}

// TestStageAcceptsTheFlowImportForm covers scenario-critic's rest.yaml, which
// writes its import inline and reaches two directories up.
func TestStageAcceptsTheFlowImportForm(t *testing.T) {
	t.Parallel()
	source := t.TempDir()
	writeDeclaration(t, source, "agents/scenario-critic/rest.yaml",
		"imports: [../../tools/rest-auth-none.yaml]\nrest:\n  version: v1\n")
	writeDeclaration(t, source, "tools/rest-auth-none.yaml", "unit: rest-auth-none\nrest:\n  version: v1\n")
	destination := t.TempDir()

	require.NoError(t, profilestage.Stage(profilestage.Tree{
		Source:      filepath.Join(source, "agents", "scenario-critic"),
		Destination: filepath.Join(destination, "agents", "scenario-critic"),
	}))

	require.FileExists(t, filepath.Join(destination, "tools", "rest-auth-none.yaml"),
		"a REST unit import is the same edge as a type unit import")
}

func TestStageReportsAnImportWithNoTarget(t *testing.T) {
	t.Parallel()
	source := t.TempDir()
	writeDeclaration(t, source, "agents/broken/declarations.yaml",
		"unit: broken\nimports:\n- ../units/absent.yaml\ntools: []\n")

	err := profilestage.Stage(profilestage.Tree{
		Source:      filepath.Join(source, "agents", "broken"),
		Destination: filepath.Join(t.TempDir(), "agents", "broken"),
	})

	require.ErrorContains(t, err, "../units/absent.yaml")
	require.ErrorContains(t, err, "declarations.yaml",
		"the error names the declaration that carries the dangling import")
}

// TestStageIgnoresNonDeclarationYAML keeps the walk from failing on the machine
// specs, tool selections, and UI config that share the staged directories.
func TestStageIgnoresNonDeclarationYAML(t *testing.T) {
	t.Parallel()
	source := t.TempDir()
	writeDeclaration(t, source, "agents/plain/machine.yaml", "name: plain\nstates: []\n")
	writeDeclaration(t, source, "agents/plain/notes.txt", "not yaml at all\n")
	destination := t.TempDir()

	require.NoError(t, profilestage.Stage(profilestage.Tree{
		Source:      filepath.Join(source, "agents", "plain"),
		Destination: filepath.Join(destination, "agents", "plain"),
	}))

	require.FileExists(t, filepath.Join(destination, "agents", "plain", "machine.yaml"))
	require.FileExists(t, filepath.Join(destination, "agents", "plain", "notes.txt"))
}
