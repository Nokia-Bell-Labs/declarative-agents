// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package service

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

func TestServiceDeclarationsMatchStandardInits(t *testing.T) {
	t.Parallel()

	defs, err := catalog.LoadToolDefs(serviceDeclarationsPath(t))
	require.NoError(t, err)

	inits := make([]string, 0, len(defs))
	for _, def := range defs {
		require.Equal(t, "builtin", def.Type, def.Name)
		require.Equal(t, "boundary", def.Category, def.Name)
		require.NotEmpty(t, def.Emits, def.Name)
		require.Contains(t, StandardInits, def.Init,
			"fixture init %q is not registered", def.Init)
		inits = append(inits, def.Init)
	}
	require.ElementsMatch(t, StandardInits, inits,
		"every registered service init must have exactly one fixture ToolDef")
}

func serviceDeclarationsPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Join(filepath.Dir(file), "testdata", "declarations.yaml")
}
