// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package rest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/corepath"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

// A machine_request loads its request profile's declarations while the agent
// runs, after the agent's own closure restored the root registry. The request
// profile's roots, with any --library override, are in force for that load, so
// a chat dialect under /opt/providers resolves (srd058 R1.1). The registries
// are process-scoped, so this test does not run in parallel.
func TestMachineRequestLoadsDeclarationsUnderItsProfileRoots(t *testing.T) {
	t.Cleanup(func() { corepath.SetLibraryRoots(nil); corepath.SetLibraryOverrides(nil) })
	agentCore, err := filepath.Abs(filepath.Join("..", "..", ".."))
	require.NoError(t, err)
	fixture := filepath.Join(agentCore, "testdata", "providers", "fixture")
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "declarations.yaml"), []byte(`tools:
  - name: ask
    type: builtin
    init: invoke_llm
    config: {dialect: /opt/providers/chat-dialect.yaml, model: m}
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "profile.yaml"), []byte(`name: request
machine: machine.yaml
tools: [tools.yaml]
tool_declarations: [declarations.yaml]
libraries: {providers: `+filepath.Join(agentCore, "tools", "providers", "ollama")+`}
`), 0o644))
	profile, err := catalog.LoadProfile(filepath.Join(dir, "profile.yaml"))
	require.NoError(t, err)

	defs, err := loadRequestDeclarations(profile)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(agentCore, "tools", "providers", "ollama", "chat-dialect.yaml"), defs[0].Config["dialect"])
	require.Empty(t, corepath.LibraryRoots(), "the roots do not outlive the load")

	corepath.SetLibraryOverrides(map[string]string{"providers": fixture})
	defs, err = loadRequestDeclarations(profile)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(fixture, "chat-dialect.yaml"), defs[0].Config["dialect"], "--library applies to request loads too")
}
