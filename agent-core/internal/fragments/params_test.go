// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package fragments_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/fragments"
)

func stringPtr(value string) *string { return &value }

func embedParams() []fragments.Param {
	return []fragments.Param{
		{Name: "provider", Type: "string", Enum: []string{"cohere", "openai"}},
		{Name: "dimensions", Type: "integer"},
		{Name: "timeout", Type: "number", Default: stringPtr("2.5")},
		{Name: "batch", Type: "boolean", Default: stringPtr("false")},
	}
}

func TestResolveArgsAppliesDefaultsAndTypesEachValue(t *testing.T) {
	t.Parallel()
	resolved, err := fragments.ResolveArgs(embedParams(),
		map[string]string{"provider": "cohere", "dimensions": "1024"})

	require.NoError(t, err)
	require.Equal(t, fragments.Arg{Value: "cohere", Tag: "!!str"}, resolved["provider"])
	require.Equal(t, fragments.Arg{Value: "1024", Tag: "!!int"}, resolved["dimensions"])
	require.Equal(t, fragments.Arg{Value: "2.5", Tag: "!!float"}, resolved["timeout"])
	require.Equal(t, fragments.Arg{Value: "false", Tag: "!!bool"}, resolved["batch"])
}

// TestResolveArgsNamesTheParameterOnEveryFault is srd052 R2.2: the caller adds
// fragment and importer, so each message here must carry the parameter.
func TestResolveArgsNamesTheParameterOnEveryFault(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		args map[string]string
		want string
	}{
		"missing required": {map[string]string{"provider": "cohere"}, `parameter "dimensions" is required`},
		"unknown argument": {map[string]string{"provider": "cohere", "dimensions": "8", "model": "x"}, `argument "model" names no declared parameter`},
		"wrong type":       {map[string]string{"provider": "cohere", "dimensions": "many"}, `parameter "dimensions": value "many" is not an integer`},
		"outside enum":     {map[string]string{"provider": "ollama", "dimensions": "8"}, `parameter "provider": value "ollama" is not one of cohere, openai`},
		"bad boolean":      {map[string]string{"provider": "cohere", "dimensions": "8", "batch": "yes"}, `parameter "batch": value "yes" is not true or false`},
	} {
		_, err := fragments.ResolveArgs(embedParams(), tc.args)
		require.ErrorContains(t, err, tc.want, name)
	}
}

func TestValidateParamsRejectsBadDeclarations(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		params []fragments.Param
		want   string
	}{
		"unknown type":   {[]fragments.Param{{Name: "n", Type: "list"}}, `type "list" is not one of`},
		"duplicate name": {[]fragments.Param{{Name: "n", Type: "string"}, {Name: "n", Type: "string"}}, `declared twice`},
		"no name":        {[]fragments.Param{{Type: "string"}}, `no name`},
		"bad default":    {[]fragments.Param{{Name: "n", Type: "integer", Default: stringPtr("x")}}, `default value "x" is not an integer`},
	} {
		require.ErrorContains(t, fragments.ValidateParams(tc.params), tc.want, name)
	}
	require.NoError(t, fragments.ValidateParams(embedParams()))
}

func TestFormatArgsIsOrdered(t *testing.T) {
	t.Parallel()
	require.Equal(t, "a=1, b=2", fragments.FormatArgs(map[string]string{"b": "2", "a": "1"}))
}
