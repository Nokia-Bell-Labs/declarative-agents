// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package spec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestContractBaselineLocationIsCodeAdjacent(t *testing.T) {
	root := repoAgentCoreRoot(t)
	require.Equal(t,
		"pkg/spec/testdata/tool-contract-completeness-baseline.yaml",
		filepath.ToSlash(ContractBaselineFile),
	)
	require.FileExists(t, filepath.Join(root, ContractBaselineFile))

	_, err := os.Stat(filepath.Join(root, "docs", "tool-contract-completeness-baseline.yaml"))
	require.Error(t, err)
	require.True(t, os.IsNotExist(err))

	_, ok, err := loadContractBaseline(root)
	require.NoError(t, err)
	require.True(t, ok)
}
