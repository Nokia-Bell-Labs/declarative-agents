// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package definition

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/corepath"
)

// Library-rooted REST imports and fragments (srd056 R1). The install root is
// process-scoped, so this test does not run in parallel.

func TestRESTImportsAndFragmentsResolveUnderTheAgentCoreLibrary(t *testing.T) {
	installRoot := t.TempDir()
	writeImportFixture(t, installRoot, "tools/units/limits.yaml",
		"unit: builtin-limits\nrest:\n  limits:\n    shared: {max_request_bytes: 1024}\n")
	fragment := writeImportFixture(t, installRoot, "tools/units/api.yaml", apiFragment)
	corepath.SetInstallRoot(installRoot)
	t.Cleanup(func() { corepath.SetInstallRoot("") })
	top := writeImportFixture(t, t.TempDir(), "rest.yaml", `unit: top
imports: [/opt/agent-core/tools/units/limits.yaml]
instantiate:
- {fragment: /opt/agent-core/tools/units/api.yaml, as: metrics, args: {base_url: "http://metrics:9090"}}
rest: {}
`)

	def, err := LoadDefinitionClosure([]string{top}, nil)

	require.NoError(t, err)
	require.Equal(t, 1024, def.Limits["shared"].MaxRequestBytes)
	source, ok := def.DeclarationSource("clients", "metrics_api")
	require.True(t, ok)
	require.Equal(t, fragment, source.Path, "the produced client records the mapped library fragment")
}
