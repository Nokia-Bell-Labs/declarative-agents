// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package fragments_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/fragments"
)

func parse(t *testing.T, text string) *yaml.Node {
	t.Helper()
	var doc yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte(text), &doc))
	return &doc
}

func encode(t *testing.T, doc *yaml.Node) string {
	t.Helper()
	out, err := yaml.Marshal(doc)
	require.NoError(t, err)
	return string(out)
}

func args() map[string]fragments.Arg {
	return map[string]fragments.Arg{
		"provider": {Value: "cohere", Tag: "!!str"},
		"dims":     {Value: "1024", Tag: "!!int"},
		"batch":    {Value: "true", Tag: "!!bool"},
	}
}

// TestSubstituteFillsScalarHolesWithTheirTypes: a whole-scalar hole takes the
// argument's type, so a decoder sees an integer where an integer parameter
// stood; a hole inside a longer string stays text.
func TestSubstituteFillsScalarHolesWithTheirTypes(t *testing.T) {
	t.Parallel()
	doc := parse(t, "tools:\n- name: embed\n  description: Embed with $param(provider) at $param(dims)\n  config:\n    dimensions: $param(dims)\n    batch: $param(batch)\n    provider: $param(provider)\n")

	require.NoError(t, fragments.Substitute(doc, args()))

	var decoded struct {
		Tools []struct {
			Description string
			Config      struct {
				Dimensions int
				Batch      bool
				Provider   string
			}
		}
	}
	require.NoError(t, yaml.Unmarshal([]byte(encode(t, doc)), &decoded))
	require.Equal(t, "Embed with cohere at 1024", decoded.Tools[0].Description)
	require.Equal(t, 1024, decoded.Tools[0].Config.Dimensions)
	require.True(t, decoded.Tools[0].Config.Batch)
	require.Equal(t, "cohere", decoded.Tools[0].Config.Provider)
}

// TestSubstituteLeavesMappingNamesAlone is the hygiene rule (srd052 R2.3): a
// reference in a mapping name is not a hole, so it survives and Leftover then
// reports it rather than an argument quietly minting a field.
func TestSubstituteLeavesMappingNamesAlone(t *testing.T) {
	t.Parallel()
	doc := parse(t, "config:\n  $param(provider)_api: x\n  provider: $param(provider)\n")

	require.NoError(t, fragments.Substitute(doc, args()))

	require.Contains(t, encode(t, doc), "$param(provider)_api")
	line, found := fragments.Leftover(doc)
	require.True(t, found)
	require.Equal(t, 2, line)
}

func TestSubstituteRejectsAnUndeclaredParameterWithItsLine(t *testing.T) {
	t.Parallel()
	whole := parse(t, "a: 1\nb: $param(missing)\n")
	require.ErrorContains(t, fragments.Substitute(whole, args()), "line 2: $param(missing) names no declared parameter")

	partial := parse(t, "a: 1\nb: 2\nc: prefix-$param(missing)-suffix\n")
	require.ErrorContains(t, fragments.Substitute(partial, args()), "line 3: $param(missing)")
}

func TestSubstituteIgnoresEnvironmentAndSelectorForms(t *testing.T) {
	t.Parallel()
	text := "a: ${HOST:-localhost}\nb: $from(label).path\nc: $.output\n"
	doc := parse(t, text)

	require.NoError(t, fragments.Substitute(doc, args()))

	require.Equal(t, text, encode(t, doc))
	_, found := fragments.Leftover(doc)
	require.False(t, found)
	require.False(t, fragments.References([]byte(text)))
	require.True(t, fragments.References([]byte("x: $param(y)")))
}

func TestRemoveFieldDropsOneTopLevelEntry(t *testing.T) {
	t.Parallel()
	doc := parse(t, "unit: f\nparams:\n- name: p\n  type: string\ntools: []\n")

	fragments.RemoveField(doc, "params")

	require.Equal(t, "unit: f\ntools: []\n", encode(t, doc))
}
