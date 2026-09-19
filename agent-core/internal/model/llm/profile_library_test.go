// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package llm

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func libraryProfiles(t *testing.T, text string) []ProfileSpec {
	t.Helper()
	var specs []ProfileSpec
	require.NoError(t, yaml.Unmarshal([]byte(text), &specs))
	return specs
}

func TestLibraryParserProfileOverridesEmbedded(t *testing.T) {
	t.Parallel()
	embedded, err := DefaultProfileRegistry()
	require.NoError(t, err)
	specs := libraryProfiles(t, `
- name: cohere
  match_prefixes: ["command-"]
  envelope: {open: "<call>", close: "</call>"}
  extraction_pipeline:
    - extract_envelope: {open: "<call>", close: "</call>"}
- name: acme
  match_prefixes: ["acme-"]
  extraction_pipeline:
    - extract_braces
`)

	merged, err := embedded.WithLibrary("chat-dialect.yaml", specs)
	require.NoError(t, err)

	byName, ok := merged.ResolveProfileName("cohere")
	require.True(t, ok)
	require.Equal(t, `{"a":1}`, byName.ExtractToolCall(`<call>{"a":1}</call>`),
		"a library profile replaces the embedded profile of the same name")
	require.Equal(t, "cohere", merged.resolveProfileSpec("command-a-03-2025").ProfileName)
	require.Equal(t, "acme", merged.resolveProfileSpec("acme-large").ProfileName, "a library-only profile resolves by prefix")
	require.Equal(t, "qwen", merged.resolveProfileSpec("qwen3:8b").ProfileName, "embedded profiles the library does not replace stay")

	original, ok := embedded.ResolveProfileName("cohere")
	require.True(t, ok)
	require.Equal(t, `{"a":1}`, original.ExtractToolCall(`[tool_call]{"a":1}[/tool_call]`),
		"the embedded registry is not modified")
}

func TestLibraryParserProfilesAreValidated(t *testing.T) {
	t.Parallel()
	embedded, err := DefaultProfileRegistry()
	require.NoError(t, err)

	_, err = embedded.WithLibrary("d.yaml", libraryProfiles(t, "- match_prefixes: [x-]\n"))
	require.ErrorContains(t, err, "missing 'name'")

	_, err = embedded.WithLibrary("d.yaml", libraryProfiles(t, `
- {name: one, match_prefixes: [acme-]}
- {name: two, match_prefixes: [acme-large]}
`))
	require.ErrorContains(t, err, "ambiguous")
}
