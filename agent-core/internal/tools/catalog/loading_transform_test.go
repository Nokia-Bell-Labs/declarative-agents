// Copyright (c) 2026 Nokia. All rights reserved.

package catalog

import (
	"encoding/json"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestStripCLIExtensions(t *testing.T) {
	t.Parallel()

	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"msg": map[string]interface{}{
				"type":        "string",
				"description": "Message",
				"flag":        "-m",
				"positional":  false,
				"bool_flag":   false,
				"default":     "hello",
				"position":    1,
			},
		},
		"required": []interface{}{"msg"},
	}

	td := ToolDef{Name: "clean", Binary: "true", Parameters: schema}
	var cleaned map[string]interface{}
	require.NoError(t, json.Unmarshal(td.ToToolSpec().InputSchema, &cleaned))

	props := cleaned["properties"].(map[string]interface{})
	msg := props["msg"].(map[string]interface{})

	assert.Equal(t, "string", msg["type"])
	assert.Equal(t, "Message", msg["description"])
	assert.NotContains(t, msg, "flag")
	assert.NotContains(t, msg, "positional")
	assert.NotContains(t, msg, "bool_flag")
	assert.NotContains(t, msg, "default")
	assert.NotContains(t, msg, "position")

	assert.Contains(t, cleaned, "required")
	assert.Equal(t, "object", cleaned["type"])
}

func TestMergeToolDefs(t *testing.T) {
	t.Parallel()

	base := []ToolDef{
		{
			Name:     "build",
			Binary:   "go",
			Args:     []string{"build"},
			Category: "boundary",
			Problem:  "Compile the workspace.",
			Reversibility: ToolReversibility{
				Classification: "reversible",
				Undo:           "noop",
			},
		},
		{Name: "test", Binary: "go", Args: []string{"test"}},
	}
	override := []ToolDef{
		{
			Name:     "build",
			Binary:   "go",
			Args:     []string{"build", "-race"},
			Category: "boundary",
			Problem:  "Compile the workspace with race detector.",
			Reversibility: ToolReversibility{
				Classification: "reversible",
				Undo:           "noop",
			},
		},
		{Name: "lint", Binary: "golangci-lint"},
	}

	merged := MergeToolDefs(base, override)
	assert.Len(t, merged, 3)

	assert.Equal(t, "build", merged[0].Name)
	assert.Equal(t, []string{"build", "-race"}, merged[0].Args)
	assert.Equal(t, "boundary", merged[0].Category)
	assert.Equal(t, "Compile the workspace with race detector.", merged[0].Problem)
	assert.Equal(t, "reversible", merged[0].Reversibility.Classification)
	assert.Equal(t, "test", merged[1].Name)
	assert.Equal(t, "lint", merged[2].Name)
}

func TestExtractParamMappings_Empty(t *testing.T) {
	t.Parallel()

	td := ToolDef{Name: "noop", Binary: "true"}
	assert.Nil(t, td.ExtractParamMappings())
}

func TestExtractParamMappings_Full(t *testing.T) {
	t.Parallel()

	td := ToolDef{
		Name:   "test",
		Binary: "go",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"pkg": map[string]interface{}{
					"type":       "string",
					"positional": true,
					"default":    "./...",
				},
				"verbose": map[string]interface{}{
					"type":      "boolean",
					"flag":      "-v",
					"bool_flag": true,
				},
			},
			"required": []interface{}{"pkg"},
		},
	}

	mappings := td.ExtractParamMappings()
	assert.Len(t, mappings, 2)

	pkg := findMapping(mappings, "pkg")
	require.NotNil(t, pkg)
	assert.True(t, pkg.Positional)
	assert.True(t, pkg.Required)
	assert.Equal(t, "./...", pkg.Default)

	verbose := findMapping(mappings, "verbose")
	require.NotNil(t, verbose)
	assert.True(t, verbose.BoolFlag)
	assert.Equal(t, "-v", verbose.Flag)
	assert.False(t, verbose.Required)
}

func TestExtractParamMappingsUsesPositionThenStableName(t *testing.T) {
	t.Parallel()
	td := ToolDef{Parameters: map[string]interface{}{
		"properties": map[string]interface{}{
			"zeta":        map[string]interface{}{"positional": true},
			"destination": map[string]interface{}{"positional": true, "position": 3},
			"verbose":     map[string]interface{}{"flag": "-v", "bool_flag": true, "position": 2},
			"source":      map[string]interface{}{"positional": true, "position": 1},
			"alpha":       map[string]interface{}{"flag": "--alpha"},
		},
	}}
	want := []string{"source", "verbose", "destination", "alpha", "zeta"}

	for range 500 {
		mappings := td.ExtractParamMappings()
		names := make([]string, 0, len(mappings))
		for _, mapping := range mappings {
			names = append(names, mapping.Name)
		}
		assert.Equal(t, want, names)
	}
}
