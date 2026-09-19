// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package load

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/corepath"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

// Rebinding --library providers switches provider with no declaration edit
// (srd058 AC1, AC4). The ollama-rest integration profile is loaded unchanged
// against the shipped Ollama and Cohere libraries and the synthetic
// OpenAI-shaped one; only the resolved dialect, and so the program identity,
// differs. The root registries are process-scoped, so these tests do not run
// in parallel.

func ollamaRestProfile(t *testing.T) string {
	t.Helper()
	return filepath.Join(agentCoreRoot(t), "testdata", "integration", "profiles", "ollama-rest", "profile.yaml")
}

func loadWithProviders(t *testing.T, library string) *Closure {
	t.Helper()
	corepath.SetLibraryOverrides(map[string]string{"providers": library})
	closure, err := LoadClosure(ollamaRestProfile(t), Options{})
	require.NoError(t, err, "bound to %s", library)
	return closure
}

func TestLibraryRebindSwitchesProvider(t *testing.T) {
	t.Cleanup(func() { corepath.SetLibraryRoots(nil); corepath.SetLibraryOverrides(nil) })
	core := agentCoreRoot(t)
	libraries := map[string]string{
		"ollama":        filepath.Join(core, "tools", "providers", "ollama"),
		"cohere":        filepath.Join(core, "tools", "providers", "cohere"),
		"openai-shaped": filepath.Join(core, "testdata", "providers", "openai-shaped"),
	}
	declarations := map[string][]byte{}
	identities := map[string]string{}
	for name, library := range libraries {
		closure := loadWithProviders(t, library)

		dialect := filepath.Join(library, "chat-dialect.yaml")
		require.Equal(t, dialect, closure.Selected[indexOfTool(t, closure.Selected, "invoke_llm")].Config["dialect"],
			"%s: invoke_llm reads the bound library's dialect", name)
		require.Contains(t, closure.Files, dialect)
		var dump bytes.Buffer
		require.NoError(t, DumpConfig(closure, &dump))
		require.Contains(t, dump.String(), dialect, "%s: the dump names the dialect", name)

		identities[name] = catalog.BuildProgramRefFromAssets(closure.ProfilePath, closure.Assets).Digest
		for path, data := range closure.Assets {
			if strings.HasPrefix(path, library) {
				continue
			}
			if previous, seen := declarations[path]; seen {
				require.Equal(t, previous, data, "%s: %s is byte-identical under every binding", name, path)
			}
			declarations[path] = data
		}
	}
	require.NotEqual(t, identities["ollama"], identities["cohere"])
	require.NotEqual(t, identities["ollama"], identities["openai-shaped"])
	require.NotEqual(t, identities["cohere"], identities["openai-shaped"])
}

// The synthetic provider is a directory of YAML: adding it is the directory,
// and no Go or agent declaration names it.
func TestSyntheticProviderTouchesOnlyItsLibrary(t *testing.T) {
	t.Cleanup(func() { corepath.SetLibraryRoots(nil); corepath.SetLibraryOverrides(nil) })
	core := agentCoreRoot(t)
	library := filepath.Join(core, "testdata", "providers", "openai-shaped")
	entries, err := os.ReadDir(library)
	require.NoError(t, err)
	for _, entry := range entries {
		require.Equal(t, ".yaml", filepath.Ext(entry.Name()), "the library holds declarations only")
	}

	closure := loadWithProviders(t, library)

	for path := range closure.Assets {
		if strings.HasPrefix(path, library) {
			continue
		}
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		require.NotContains(t, string(data), "openai", "%s names the synthetic provider", path)
	}
}

func indexOfTool(t *testing.T, tools []catalog.ToolDef, name string) int {
	t.Helper()
	for index, tool := range tools {
		if tool.Name == name {
			return index
		}
	}
	t.Fatalf("no selected tool %s", name)
	return -1
}
