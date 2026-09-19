// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package load

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/corepath"
)

// A declaration that still names a legacy provider runs with the shipped
// library's dialect, and that file is a closure file like a declared dialect:
// in the program identity and in the dump (srd058 R2.3, R3.2; GH-2248). The
// install root is process-scoped, so these tests do not run in parallel.

func writeLegacyProviderProfile(t *testing.T, config string) string {
	t.Helper()
	root := writeUsednessClosureFixture(t, "ask")
	writeLoadFixture(t, root, "declarations.yaml",
		"tools:\n  - name: ask\n    type: builtin\n    init: invoke_llm\n    config: {"+config+"}\n")
	return filepath.Join(root, "profile.yaml")
}

func TestLegacyProviderDialectJoinsTheClosure(t *testing.T) {
	core := agentCoreRoot(t)
	corepath.SetInstallRoot(core)
	t.Cleanup(func() { corepath.SetInstallRoot("") })
	for provider, config := range map[string]string{
		"ollama": "model: m",
		"cohere": "provider: cohere, provider_url: https://api.cohere.com, model: m",
	} {
		dialect := filepath.Join(core, "tools", "providers", provider, "chat-dialect.yaml")

		closure, err := LoadClosure(writeLegacyProviderProfile(t, config), Options{})

		require.NoError(t, err, provider)
		require.Contains(t, closure.Files, dialect, "%s: the shipped dialect is a closure file", provider)
		tool := closure.Selected[0].Config
		require.Equal(t, dialect, tool["dialect"])
		require.NotContains(t, tool, "provider", "the rewritten config names the dialect only")
		var dump bytes.Buffer
		require.NoError(t, DumpConfig(closure, &dump))
		require.Contains(t, dump.String(), dialect)
	}
}

func TestLegacyOllamaConfigKeepsItsLocalDefault(t *testing.T) {
	corepath.SetInstallRoot(agentCoreRoot(t))
	t.Cleanup(func() { corepath.SetInstallRoot("") })

	closure, err := LoadClosure(writeLegacyProviderProfile(t, "model: m"), Options{})

	require.NoError(t, err)
	require.Equal(t, "http://localhost:11434", closure.Selected[0].Config["provider_url"])
}

func TestLegacyProviderIsLeftAloneWithoutTheShippedLibrary(t *testing.T) {
	corepath.SetInstallRoot(t.TempDir())
	t.Cleanup(func() { corepath.SetInstallRoot("") })

	closure, err := LoadClosure(writeLegacyProviderProfile(t, "provider: ollama, model: m"), Options{})

	require.NoError(t, err)
	require.Equal(t, "ollama", closure.Selected[0].Config["provider"], "the runtime resolves or reports it")
	require.NotContains(t, closure.Selected[0].Config, "dialect")
}
