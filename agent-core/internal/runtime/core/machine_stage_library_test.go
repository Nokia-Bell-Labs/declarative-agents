// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package core_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/corepath"
)

// TestLoadMachineClosureSplicesAStageFromTheAgentCoreLibrary is srd056 R1.2 for
// machine stages. The install root is process-scoped, so it does not run in
// parallel.
func TestLoadMachineClosureSplicesAStageFromTheAgentCoreLibrary(t *testing.T) {
	installRoot := writeStageFixture(t, map[string]string{"tools/machines/run.yaml": runStage})
	corepath.SetInstallRoot(installRoot)
	t.Cleanup(func() { corepath.SetInstallRoot("") })
	root := writeStageFixture(t, map[string]string{
		"machine.yaml": machineHead + `instantiate:
  - {fragment: /opt/agent-core/tools/machines/run.yaml, args: {prefix: Embed, enter: Embed, word: embed_query}}
transitions:
  - {state: Ready, signal: Seed, next: Done}
`,
	})

	spec, err := core.LoadMachineClosure(filepath.Join(root, "machine.yaml"), nil)

	require.NoError(t, err)
	require.Contains(t, spec.States.Names(), "EmbedRunning")
	require.Equal(t, filepath.Join(installRoot, "tools", "machines", "run.yaml"), spec.Instantiations()[0].Fragment)
}
