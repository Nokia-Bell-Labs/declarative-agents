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
		"unit: one\ninstantiate:\n- {fragment: ../units/serve.yaml, args: {word: go}}\n")
	writeDeclaration(t, source, "agents/units/serve.yaml",
		"unit: serve\nparams:\n- {name: word, type: string}\nmachine:\n  name: serve\n  instantiate:\n  - {fragment: stages/run.yaml, args: {word: $param(word)}}\n")
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
