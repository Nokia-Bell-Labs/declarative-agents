// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package catalog

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/corepath"
)

// Library-rooted tool imports and fragments (srd056 R1). The install root is
// process-scoped, so these tests do not run in parallel.

func TestToolImportsAndFragmentsResolveUnderTheAgentCoreLibrary(t *testing.T) {
	installRoot := t.TempDir()
	library := writeToolImportFixture(t, installRoot, "tools/units/words.yaml", toolUnit("builtin-words", "", "shared_word"))
	fragment := writeToolImportFixture(t, installRoot, "tools/units/embed.yaml", embedFragment)
	corepath.SetInstallRoot(installRoot)
	t.Cleanup(func() { corepath.SetInstallRoot("") })
	top := writeToolImportFixture(t, t.TempDir(), "agents/rag/declarations.yaml", `unit: rag
imports: [/opt/agent-core/tools/units/words.yaml]
instantiate:
- {fragment: /opt/agent-core/tools/units/embed.yaml, as: cohere, args: {provider: cohere}}
tools: []
`)

	resolver := newToolImportResolver(nil)
	defs, err := resolver.loadRoots([]string{top})

	require.NoError(t, err)
	require.ElementsMatch(t, []string{"shared_word", "cohere_embed"}, toolNames(defs))
	require.Equal(t, library, resolver.imports[0].Imported.Path, "the edge records the mapped library file")
	require.Equal(t, filepath.Clean(fragment), defs[1].DeclarationSource().Path)
}

func TestToolFragmentOutsideALibraryRootIsRejected(t *testing.T) {
	elsewhere := writeToolImportFixture(t, t.TempDir(), "embed.yaml", embedFragment)
	top := writeToolImportFixture(t, t.TempDir(), "declarations.yaml",
		"unit: rag\ninstantiate:\n- {fragment: "+elsewhere+", args: {provider: cohere}}\ntools: []\n")

	_, err := newToolImportResolver(nil).loadRoots([]string{top})

	require.ErrorIs(t, err, corepath.ErrOutsideLibraryRoot)
}
