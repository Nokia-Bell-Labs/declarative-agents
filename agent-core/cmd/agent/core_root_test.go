// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/pkg/spec"
)

func TestRunIgnoresAmbientAgentCoreRoot(t *testing.T) {
	snapshot := snapshotAgentFlags()
	t.Cleanup(func() {
		restoreAgentFlags(snapshot)
		spec.SetAgentCoreInstallRoot(repoRootFromRuntime())
	})
	clearAgentFlags()
	flagValidateConfig = true
	flagProfile = "missing-profile.yaml"
	spec.SetAgentCoreInstallRoot("")
	t.Setenv("AGENT_CORE_ROOT", t.TempDir())

	cmd := commandWithCoreRootFlag()
	err := run(cmd, nil)

	require.Error(t, err)
	require.Empty(t, spec.AgentCoreInstallRoot())
}

func TestRunAppliesExplicitCoreRootFlag(t *testing.T) {
	snapshot := snapshotAgentFlags()
	t.Cleanup(func() {
		restoreAgentFlags(snapshot)
		spec.SetAgentCoreInstallRoot(repoRootFromRuntime())
	})
	clearAgentFlags()
	flagValidateConfig = true
	flagProfile = "missing-profile.yaml"
	spec.SetAgentCoreInstallRoot("")
	coreRoot := t.TempDir()

	cmd := commandWithCoreRootFlag()
	require.NoError(t, cmd.Flags().Set("core-root", coreRoot))
	err := run(cmd, nil)

	require.Error(t, err)
	require.Equal(t, coreRoot, spec.AgentCoreInstallRoot())
}

func commandWithCoreRootFlag() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().StringVar(&flagCoreRoot, "core-root", "", "")
	return cmd
}
