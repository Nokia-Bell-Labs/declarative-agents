// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package llm

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

// Pinning test for application-dsl-spec 1.0 R-LLM-001: an invoke_llm config
// names its model and the state whose tools form the manifest.
func TestSpecPinInvokeLLMConfigRequiresModelAndManifestState(t *testing.T) {
	t.Parallel()
	_, err := DecodeInvokeLLMConfig(catalog.ToolDef{Config: map[string]interface{}{
		"manifest_state": "Composing",
	}})
	require.ErrorContains(t, err, "requires model")

	_, err = DecodeInvokeLLMConfig(catalog.ToolDef{Config: map[string]interface{}{
		"model": "m",
	}})
	require.ErrorContains(t, err, "requires manifest_state")

	_, err = DecodeInvokeLLMConfig(catalog.ToolDef{Config: map[string]interface{}{
		"model": "m", "manifest_state": "Composing",
	}})
	require.NoError(t, err)
}
