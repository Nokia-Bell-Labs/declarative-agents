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

	require.NoError(t, profilestage.Stage(destination, profilestage.Tree{
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

	require.NoError(t, profilestage.Stage(destination, profilestage.Tree{
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

	require.NoError(t, profilestage.Stage(destination, profilestage.Tree{
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

	require.NoError(t, profilestage.Stage(destination, profilestage.Tree{
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

	stageRoot := t.TempDir()
	err := profilestage.Stage(stageRoot, profilestage.Tree{
		Source:      filepath.Join(source, "agents", "broken"),
		Destination: filepath.Join(stageRoot, "agents", "broken"),
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

	require.NoError(t, profilestage.Stage(destination, profilestage.Tree{
		Source:      filepath.Join(source, "agents", "plain"),
		Destination: filepath.Join(destination, "agents", "plain"),
	}))

	require.FileExists(t, filepath.Join(destination, "agents", "plain", "machine.yaml"))
	require.FileExists(t, filepath.Join(destination, "agents", "plain", "notes.txt"))
}

func TestImportedReturnsWhatTheDirectoryDoesNotContain(t *testing.T) {
	t.Parallel()
	source := agentImportingAUnit(t)

	imported, err := profilestage.Imported(filepath.Join(source, "agents", "collector"))

	require.NoError(t, err)
	require.Equal(t,
		[]string{filepath.Join(source, "agents", "units", "types-core.yaml")}, imported,
		"a walk of the agent directory never reaches the unit beside it")
}

func TestImportedOmitsWhatTheDirectoryAlreadyHolds(t *testing.T) {
	t.Parallel()
	source := t.TempDir()
	writeDeclaration(t, source, "agents/collector/declarations.yaml",
		"unit: collector\nimports:\n- local-unit.yaml\ntools: []\n")
	writeDeclaration(t, source, "agents/collector/local-unit.yaml", "unit: local\ntypes: []\n")

	imported, err := profilestage.Imported(filepath.Join(source, "agents", "collector"))

	require.NoError(t, err)
	require.Empty(t, imported, "a caller walking the directory already sees a sibling inside it")
}

func TestImportedFollowsTheChainOutOfTheDirectory(t *testing.T) {
	t.Parallel()
	source := t.TempDir()
	writeDeclaration(t, source, "agents/collector/declarations.yaml",
		"unit: collector\nimports:\n- ../units/types-core.yaml\ntools: []\n")
	writeDeclaration(t, source, "agents/units/types-core.yaml",
		"unit: types-core\nimports:\n- ../../shared/types-shared.yaml\ntypes: []\n")
	writeDeclaration(t, source, "shared/types-shared.yaml", "unit: types-shared\ntypes: []\n")

	imported, err := profilestage.Imported(filepath.Join(source, "agents", "collector"))

	require.NoError(t, err)
	require.Equal(t, []string{
		filepath.Join(source, "agents", "units", "types-core.yaml"),
		filepath.Join(source, "shared", "types-shared.yaml"),
	}, imported, "the chain is followed past the first hop and returned in a stable order")
}

// TestStageRefusesAnImportThatLeavesTheRoot is GH-2075. An import deep enough
// to climb out of the staged root used to land beside it and report success,
// and Validate could not tell, because the copy sat exactly where the staged
// declaration resolved it. The tree loaded on the machine that staged it and
// failed only once the root was used on its own.
func TestStageRefusesAnImportThatLeavesTheRoot(t *testing.T) {
	t.Parallel()
	source := t.TempDir()
	writeDeclaration(t, source, "agents/probe/declarations.yaml",
		"unit: probe\nimports:\n- ../../shared/types.yaml\ntools: []\n")
	writeDeclaration(t, source, "shared/types.yaml", "unit: shared-types\ntypes: []\n")
	parent := t.TempDir()
	root := filepath.Join(parent, "stage")

	err := profilestage.Stage(root, profilestage.Tree{
		Source:      filepath.Join(source, "agents", "probe"),
		Destination: filepath.Join(root, "agents"),
	})

	require.ErrorContains(t, err, "../../shared/types.yaml")
	require.ErrorContains(t, err, "outside the staged root")
	require.NoFileExists(t, filepath.Join(parent, "shared", "types.yaml"),
		"nothing is written beside the root")
}

// TestStagePlacesAnImportOutsideItsTreeButInsideTheRoot keeps what GH-2041
// designed: a unit beside an agent directory lands beside its copy, outside the
// tree that declares it. The chatbot-mesh embedding-exclusion stager depends on
// exactly this, staging agents/chatbot as one tree and walking the root for the
// units it imports.
func TestStagePlacesAnImportOutsideItsTreeButInsideTheRoot(t *testing.T) {
	t.Parallel()
	source := agentImportingAUnit(t)
	root := t.TempDir()

	require.NoError(t, profilestage.Stage(root, profilestage.Tree{
		Source:      filepath.Join(source, "agents", "collector"),
		Destination: filepath.Join(root, "collector-shifted"),
	}))

	require.FileExists(t, filepath.Join(root, "units", "types-core.yaml"))
}

func TestStageRefusesATreeDestinationOutsideTheRoot(t *testing.T) {
	t.Parallel()
	source := agentImportingAUnit(t)

	err := profilestage.Stage(t.TempDir(), profilestage.Tree{
		Source:      filepath.Join(source, "agents", "collector"),
		Destination: filepath.Join(t.TempDir(), "collector"),
	})

	require.ErrorContains(t, err, "outside the staged root")
}

// TestStageCarriesAnInstantiatedFragment: an instantiation is an import edge
// whose unit is filled in on the way (srd052 R2.1), so the fragment travels
// with the declaration that instantiates it, and a machine's stage fragment
// travels with the machine.
func TestStageCarriesAnInstantiatedFragment(t *testing.T) {
	t.Parallel()
	source := t.TempDir()
	writeDeclaration(t, source, "agents/rag/declarations.yaml",
		"unit: rag\nexpand:\n- {fragment: ../units/embed.yaml, args: {provider: cohere}}\ntools: []\n")
	writeDeclaration(t, source, "agents/units/embed.yaml",
		"unit: embed\nparams:\n- {name: provider, type: string}\ntools: []\n")
	writeDeclaration(t, source, "agents/rag/machine.yaml",
		"name: rag\nexpand:\n- {fragment: ../units/stage.yaml, args: {prefix: Embed}}\nstates: []\n")
	writeDeclaration(t, source, "agents/units/stage.yaml",
		"unit: stage\nparams:\n- {name: prefix, type: string}\nstage: {transitions: []}\n")
	root := t.TempDir()

	require.NoError(t, profilestage.Stage(root, profilestage.Tree{
		Source:      filepath.Join(source, "agents", "rag"),
		Destination: filepath.Join(root, "agents", "rag"),
	}))

	require.FileExists(t, filepath.Join(root, "agents", "units", "embed.yaml"),
		"the tool fragment the declaration instantiates travels with it")
	require.FileExists(t, filepath.Join(root, "agents", "units", "stage.yaml"),
		"the stage fragment the machine instantiates travels with it")
	imported, err := profilestage.Imported(filepath.Join(source, "agents", "rag"))
	require.NoError(t, err)
	require.ElementsMatch(t, []string{
		filepath.Join(source, "agents", "units", "embed.yaml"),
		filepath.Join(source, "agents", "units", "stage.yaml"),
	}, imported, "Imported reports fragments beside imports")
}

// installedUIDependencies plants what npm leaves beside an agent's UI: a .bin
// symlink to a file, a symlink to a directory (a file: dependency), and a
// dependency's own YAML, which is not a declaration (GH-2277).
func installedUIDependencies(t *testing.T, agent string) {
	t.Helper()
	modules := filepath.Join(agent, "ui", "node_modules")
	writeDeclaration(t, modules, "dep/package.yaml", "imports:\n- ../../missing.yaml\n")
	writeDeclaration(t, agent, "ui/dist/index.html", "<html></html>")
	require.NoError(t, os.MkdirAll(filepath.Join(modules, ".bin"), 0o755))
	require.NoError(t, os.Symlink(filepath.Join(modules, "dep", "package.yaml"), filepath.Join(modules, ".bin", "tool")))
	require.NoError(t, os.MkdirAll(filepath.Join(modules, "@scope"), 0o755))
	require.NoError(t, os.Symlink(filepath.Join(agent, "ui", "dist"), filepath.Join(modules, "@scope", "kit")))
}

func TestStageSkipsInstalledUIDependencies(t *testing.T) {
	t.Parallel()
	source := agentImportingAUnit(t)
	agent := filepath.Join(source, "agents", "collector")
	installedUIDependencies(t, agent)
	destination := t.TempDir()

	require.NoError(t, profilestage.Stage(destination, profilestage.Tree{
		Source:      agent,
		Destination: filepath.Join(destination, "agents", "collector"),
	}))

	require.FileExists(t, filepath.Join(destination, "agents", "collector", "ui", "dist", "index.html"),
		"the built UI still travels with its agent")
	require.NoDirExists(t, filepath.Join(destination, "agents", "collector", "ui", "node_modules"))
	require.FileExists(t, filepath.Join(destination, "agents", "units", "types-core.yaml"))
}

func TestImportedSkipsInstalledUIDependencies(t *testing.T) {
	t.Parallel()
	source := agentImportingAUnit(t)
	agent := filepath.Join(source, "agents", "collector")
	installedUIDependencies(t, agent)

	imported, err := profilestage.Imported(agent)
	require.NoError(t, err, "a dependency's YAML under node_modules is not a declaration to follow")
	require.Equal(t, []string{filepath.Join(source, "agents", "units", "types-core.yaml")}, imported)
}
