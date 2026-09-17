// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/corepath"
)

// --library overrides a declared root for one run and refuses a root the
// profile never declares (srd056 R2.2).
func TestLibraryFlagOverridesADeclaredRootOnly(t *testing.T) {
	snapshot := snapshotAgentFlags()
	t.Cleanup(func() {
		restoreAgentFlags(snapshot)
		corepath.SetLibraryOverrides(nil)
	})
	clearAgentFlags()
	flagProfile = profilePathFromTest(t, "library-one/profile.yaml")

	require.NoError(t, applyLibraryOverrides([]string{"shared=" + t.TempDir()}))
	require.Contains(t, corepath.LibraryOverrides(), "shared")

	err := applyLibraryOverrides([]string{"absent=" + t.TempDir()})
	require.ErrorContains(t, err, `declares no library root "absent"`)

	require.ErrorContains(t, applyLibraryOverrides([]string{"shared"}), "want name=path")
}
