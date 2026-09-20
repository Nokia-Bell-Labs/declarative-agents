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

// TestStageLeavesAgentCoreLibraryImportsToTheImage is srd056 R3.3: an import
// under /opt/agent-core names a file the runtime image installs, so staging
// neither copies it nor fails for want of it in the source tree.
func TestStageLeavesAgentCoreLibraryImportsToTheImage(t *testing.T) {
	t.Parallel()
	source := t.TempDir()
	writeDeclaration(t, source, "agents/collector/declarations.yaml",
		"unit: collector\nimports:\n- /opt/agent-core/tools/units/types-core.yaml\n- ../units/types-collector.yaml\ntools: []\n")
	writeDeclaration(t, source, "agents/units/types-collector.yaml",
		"unit: types-collector\ntypes:\n- name: Row\n  schema:\n    type: string\n")
	destination := t.TempDir()

	require.NoError(t, profilestage.Stage(destination, profilestage.Tree{
		Source:      filepath.Join(source, "agents", "collector"),
		Destination: filepath.Join(destination, "agents", "collector"),
	}))

	require.FileExists(t, filepath.Join(destination, "agents", "units", "types-collector.yaml"),
		"a relative import still travels with the staged declaration")
	_, err := os.Stat(filepath.Join(destination, "opt"))
	require.True(t, os.IsNotExist(err), "nothing of agent-core's library is written into the staged tree")

	imported, err := profilestage.Imported(filepath.Join(source, "agents", "collector"))
	require.NoError(t, err)
	require.Equal(t, []string{filepath.Join(source, "agents", "units", "types-collector.yaml")}, imported)
}

// TestStageFollowsMachineTemplateEdges is srd054 R3.3: an instance's template
// and the stages the template's body splices travel with the staged tree.
func TestStageFollowsMachineTemplateEdges(t *testing.T) {
	t.Parallel()
	source := t.TempDir()
	writeDeclaration(t, source, "agents/one/machine.yaml",
		"unit: one\nexpand:\n- {fragment: ../units/serve.yaml, args: {word: go}}\n")
	writeDeclaration(t, source, "agents/units/serve.yaml",
		"unit: serve\nparams:\n- {name: word, type: string}\nmachine:\n  name: serve\n  expand:\n  - {fragment: stages/run.yaml, args: {word: $param(word)}}\n")
	writeDeclaration(t, source, "agents/units/stages/run.yaml", "unit: run\nparams:\n- {name: word, type: string}\nstage: {}\n")
	destination := t.TempDir()

	require.NoError(t, profilestage.Stage(destination, profilestage.Tree{
		Source:      filepath.Join(source, "agents", "one"),
		Destination: filepath.Join(destination, "agents", "one"),
	}))

	require.FileExists(t, filepath.Join(destination, "agents", "units", "serve.yaml"))
	require.FileExists(t, filepath.Join(destination, "agents", "units", "stages", "run.yaml"),
		"a stage the template body splices resolves against the template and travels too")
}

// TestStageCopiesADeclaredLibraryRootToItsStagedPosition is srd056 R3.3: a
// file reached through a declared root lands where the staged profile's
// declared directory resolves, so the staged profile loads with no --library.
func TestStageCopiesADeclaredLibraryRootToItsStagedPosition(t *testing.T) {
	t.Parallel()
	source := t.TempDir()
	writeDeclaration(t, source, "agents/one/profile.yaml",
		"name: one\nmachine: machine.yaml\ntools: [tools.yaml]\ntool_declarations: [declarations.yaml]\nlibraries: {shared: ../../lib}\n")
	writeDeclaration(t, source, "agents/one/declarations.yaml",
		"unit: one\nimports:\n- /opt/shared/units/words.yaml\ntools: []\n")
	writeDeclaration(t, source, "lib/units/words.yaml",
		"unit: words\nimports:\n- ../types.yaml\ntools: []\n")
	writeDeclaration(t, source, "lib/types.yaml", "unit: types\ntypes: []\n")
	destination := t.TempDir()

	require.NoError(t, profilestage.Stage(destination, profilestage.Tree{
		Source:      filepath.Join(source, "agents", "one"),
		Destination: filepath.Join(destination, "agents", "one"),
	}))

	require.FileExists(t, filepath.Join(destination, "lib", "units", "words.yaml"),
		"the rooted file lands where the staged profile's ../../lib resolves")
	require.FileExists(t, filepath.Join(destination, "lib", "types.yaml"),
		"the library's own relative import travels inside the root")
	imported, err := profilestage.Imported(filepath.Join(source, "agents", "one"))
	require.NoError(t, err)
	require.ElementsMatch(t, []string{
		filepath.Join(source, "lib", "units", "words.yaml"), filepath.Join(source, "lib", "types.yaml"),
	}, imported)
}

func TestStageReportsAnUndeclaredLibraryRoot(t *testing.T) {
	t.Parallel()
	source := t.TempDir()
	writeDeclaration(t, source, "agents/one/declarations.yaml",
		"unit: one\nimports:\n- /opt/nowhere/words.yaml\ntools: []\n")

	destination := t.TempDir()
	err := profilestage.Stage(destination, profilestage.Tree{
		Source: filepath.Join(source, "agents", "one"), Destination: filepath.Join(destination, "agents", "one"),
	})

	require.ErrorContains(t, err, `library root "nowhere" is declared by no staged profile`)
}

// TestStageFollowsAChatDialectThroughTheProvidersRoot is srd058 R1.2 and
// R2.3: a tool's config.dialect is an edge like an import, so a library the
// profile declares relative to itself travels with the staged tree.
func TestStageFollowsAChatDialectThroughTheProvidersRoot(t *testing.T) {
	t.Parallel()
	source := t.TempDir()
	writeDeclaration(t, source, "agents/one/profile.yaml",
		"name: one\nmachine: machine.yaml\ntools: [tools.yaml]\ntool_declarations: [declarations.yaml]\nlibraries: {providers: ../../providers/ollama}\n")
	writeDeclaration(t, source, "agents/one/declarations.yaml",
		"tools:\n- name: ask\n  type: builtin\n  init: invoke_llm\n  config: {dialect: /opt/providers/chat-dialect.yaml}\n")
	writeDeclaration(t, source, "providers/ollama/chat-dialect.yaml", "unit: ollama-chat-dialect\n")
	destination := t.TempDir()

	require.NoError(t, profilestage.Stage(destination, profilestage.Tree{
		Source:      filepath.Join(source, "agents", "one"),
		Destination: filepath.Join(destination, "agents", "one"),
	}))

	require.FileExists(t, filepath.Join(destination, "providers", "ollama", "chat-dialect.yaml"))
	imported, err := profilestage.Imported(filepath.Join(source, "agents", "one"))
	require.NoError(t, err)
	require.Equal(t, []string{filepath.Join(source, "providers", "ollama", "chat-dialect.yaml")}, imported)
}

// TestStageLeavesAnAbsoluteProvidersRootToTheImage is srd058 R1.2: a profile
// that declares the providers root at a shipped library under /opt/agent-core
// reaches files the runtime image installs, so staging copies none of them.
func TestStageLeavesAnAbsoluteProvidersRootToTheImage(t *testing.T) {
	t.Parallel()
	source := t.TempDir()
	writeDeclaration(t, source, "agents/one/profile.yaml",
		"name: one\nmachine: machine.yaml\ntools: [tools.yaml]\ntool_declarations: [declarations.yaml]\n"+
			"libraries: {providers: /opt/agent-core/tools/providers/ollama}\n")
	writeDeclaration(t, source, "agents/one/declarations.yaml",
		"tools:\n- name: ask\n  type: builtin\n  init: invoke_llm\n  config: {dialect: /opt/providers/chat-dialect.yaml}\n")
	destination := t.TempDir()

	require.NoError(t, profilestage.Stage(destination, profilestage.Tree{
		Source:      filepath.Join(source, "agents", "one"),
		Destination: filepath.Join(destination, "agents", "one"),
	}))

	_, err := os.Stat(filepath.Join(destination, "opt"))
	require.True(t, os.IsNotExist(err), "nothing of the shipped library is written into the staged tree")
	imported, err := profilestage.Imported(filepath.Join(source, "agents", "one"))
	require.NoError(t, err)
	require.Empty(t, imported)
}
