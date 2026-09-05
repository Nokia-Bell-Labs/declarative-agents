// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package llm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCohereResponseProfile(t *testing.T) {
	t.Parallel()
	registry, err := DefaultProfileRegistry()
	require.NoError(t, err)
	require.Contains(t, registry.ProfileNames(), "cohere")
	parser, ok := registry.ResolveProfileName("cohere")
	require.True(t, ok)
	require.Equal(t,
		`{"tool":"invoke_llm_fast","parameters":{}}`,
		parser.ExtractToolCall(`[tool_call]{"tool":"invoke_llm_fast","parameters":{}}[/tool_call]`),
	)
	require.Equal(t,
		`{"tool":"invoke_llm_fast","parameters":{}}`,
		parser.ExtractToolCall("```json\n{\"tool\":\"invoke_llm_fast\",\"parameters\":{}}\n```"),
	)
	spec := registry.ResolveProfileSpec("command-r7b-12-2024")
	require.Equal(t, "cohere", spec.ProfileName)
}
