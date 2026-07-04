// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func TestRegisterToolDefs(t *testing.T) {
	t.Parallel()

	defs, err := ParseToolDefs(readFixture(t, "exectool_tools.yaml"))
	require.NoError(t, err)

	reg := core.NewRegistry()
	RegisterToolDefs(reg, "/tmp", defs)

	names := reg.ExternalToolNames()
	assert.Contains(t, names, "greet")
	assert.Contains(t, names, "list_dir")

	_, ok := reg.Resolve("greet")
	assert.True(t, ok)
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	return data
}
