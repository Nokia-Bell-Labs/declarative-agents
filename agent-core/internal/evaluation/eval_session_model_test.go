// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package evaluation

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/corepath"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

// A suite profile's model label is read from its invoke_llm declaration, which
// names its chat dialect under the profile's providers root; the label is read
// with those roots in force rather than falling back to "unknown" (GH-2248).
// The registries are process-scoped, so this test does not run in parallel.
func TestExtractModelReadsADialectDeclaration(t *testing.T) {
	core, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	corepath.SetInstallRoot(core)
	t.Cleanup(func() { corepath.SetInstallRoot(""); corepath.SetLibraryRoots(nil) })
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "llm.yaml"), []byte(`tools:
  - name: invoke_llm
    type: builtin
    init: invoke_llm
    config: {dialect: /opt/providers/chat-dialect.yaml, model: suite-model, manifest_state: Composing}
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "profile.yaml"), []byte(`name: suite
machine: machine.yaml
tools: [tools.yaml]
tool_declarations: [llm.yaml]
libraries: {providers: /opt/agent-core/tools/providers/ollama}
`), 0o644))
	profile, err := catalog.LoadProfile(filepath.Join(dir, "profile.yaml"))
	require.NoError(t, err)

	require.Equal(t, "suite-model", extractModelFromProfile(profile))
}
