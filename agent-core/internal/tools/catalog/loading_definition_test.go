// Copyright (c) 2026 Nokia. All rights reserved.

package catalog

import (
	"encoding/json"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf8"
)

func TestParseToolDefs(t *testing.T) {
	t.Parallel()

	defs, err := ParseToolDefs(readFixture(t, "exectool_tools.yaml"))
	require.NoError(t, err)
	require.Len(t, defs, 2)

	assert.Equal(t, "greet", defs[0].Name)
	assert.Equal(t, "exec", defs[0].Type)
	assert.Equal(t, "echo", defs[0].Binary)
	assert.Equal(t, []string{"hello"}, defs[0].Args)
	assert.Equal(t, "scripts", defs[0].Dir)
	assert.Equal(t, "git_repo", defs[0].Precondition)
	assert.Equal(t, 25, defs[0].OutputCap)
	assert.Equal(t, "external", defs[0].Visibility)
	assert.Equal(t, []string{"ToolDone", "ToolFailed"}, defs[0].Emits)
	require.Len(t, defs[0].SideEffects.Items, 1)
	assert.Equal(t, "stdout", defs[0].SideEffects.Items[0].Kind)

	mappings := defs[0].ExtractParamMappings()
	require.Len(t, mappings, 2)

	nameMapping := findMapping(mappings, "name")
	require.NotNil(t, nameMapping)
	assert.Equal(t, "--name", nameMapping.Flag)
	assert.True(t, nameMapping.Required)

	loudMapping := findMapping(mappings, "loud")
	require.NotNil(t, loudMapping)
	assert.True(t, loudMapping.BoolFlag)

	assert.Equal(t, "list_dir", defs[1].Name)
	pathMappings := defs[1].ExtractParamMappings()
	pathMapping := findMapping(pathMappings, "path")
	require.NotNil(t, pathMapping)
	assert.True(t, pathMapping.Positional)
	assert.Equal(t, 1, pathMapping.Position)
	assert.Equal(t, ".", pathMapping.Default)
}

func TestParseToolDefs_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		yaml  string
		errIs string
	}{
		{
			name:  "missing name",
			yaml:  "tools:\n  - binary: echo",
			errIs: "no name",
		},
		{
			name:  "missing binary",
			yaml:  "tools:\n  - name: foo",
			errIs: "no binary",
		},
		{
			name:  "invalid yaml",
			yaml:  "tools: [[[",
			errIs: "parse tool defs",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := ParseToolDefs([]byte(tt.yaml))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errIs)
		})
	}
}

func TestToolDef_ToToolSpec(t *testing.T) {
	t.Parallel()

	td := ToolDef{
		Name:        "build",
		Description: "Compile stuff.",
		Binary:      "go",
		Args:        []string{"build"},
		SideEffects: ToolSideEffects{
			LegacyText: "produces binary",
		},
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"pkg": map[string]interface{}{
					"type":        "string",
					"description": "Package",
					"flag":        "--pkg",
				},
			},
			"required": []interface{}{"pkg"},
		},
	}

	spec := td.ToToolSpec()
	assert.Equal(t, "build", spec.Name)
	assert.Contains(t, spec.Description, "Compile stuff.")
	assert.Contains(t, spec.Description, "Side effects: produces binary")
	assert.Equal(t, core.External, spec.Visibility)

	var schema map[string]interface{}
	require.NoError(t, json.Unmarshal(spec.InputSchema, &schema))
	props := schema["properties"].(map[string]interface{})
	pkg := props["pkg"].(map[string]interface{})
	assert.Equal(t, "string", pkg["type"])
	assert.Equal(t, "Package", pkg["description"])
	// CLI extensions should be stripped
	assert.NotContains(t, pkg, "flag")
}

func TestParseToolDefs_ContractFields(t *testing.T) {
	t.Parallel()

	defs, err := ParseToolDefs(readFixture(t, "exectool_contract_fields.yaml"))
	require.NoError(t, err)
	require.Len(t, defs, 1)

	def := defs[0]
	assert.Equal(t, "word", def.Category)
	assert.Equal(t, "The agent needs a typed way to turn CSV files into row data.", def.Problem)
	assert.Equal(t, []string{"Return deterministic row data for a single CSV input."}, def.Goals)
	assert.Equal(t, []string{"must accept a path to one CSV file"}, def.Requirements.Input)
	assert.Equal(t, []string{"Does not transform or clean CSV values."}, def.NonGoals)
	assert.Equal(t, "Parsed CSV rows.", def.Output.Description)
	assert.Equal(t, "object", def.Output.Schema["type"])
	require.Len(t, def.SideEffects.Items, 1)
	assert.Equal(t, "none", def.SideEffects.Items[0].Kind)
	assert.Equal(t, "reversible", def.Reversibility.Classification)
	assert.Equal(t, "noop", def.Undo.Strategy)
	require.Len(t, def.Errors, 1)
	assert.Equal(t, "ToolFailed", def.Errors[0].Signal)
	assert.Equal(t, []string{"read"}, def.Relationships.Before)
	require.Len(t, def.Relationships.Overlaps, 1)
	assert.Equal(t, "read", def.Relationships.Overlaps[0].Tool)
}

func TestParseToolDefs_LegacySideEffectsString(t *testing.T) {
	t.Parallel()

	defs, err := ParseToolDefs([]byte(`tools:
  - name: copy_dir
    type: exec
    binary: cp
    description: "Copy directory"
    side_effects: "creates files in the destination directory"
`))
	require.NoError(t, err)
	require.Len(t, defs, 1)
	assert.Equal(t, "creates files in the destination directory", defs[0].SideEffects.LegacyText)
	assert.Empty(t, defs[0].SideEffects.Items)
}

func TestToolSideEffectsYAMLRoundTrip(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		yaml string
	}{
		{name: "legacy scalar", yaml: "    side_effects: creates files\n"},
		{name: "structured list", yaml: "    side_effects:\n      - kind: filesystem_write\n        target: workspace\n"},
		{name: "empty list", yaml: "    side_effects: []\n"},
		{name: "omitted", yaml: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			input := "tools:\n  - name: sample\n    binary: echo\n" + tt.yaml
			defs, err := ParseToolDefs([]byte(input))
			require.NoError(t, err)
			require.Len(t, defs, 1)
			encoded, err := yaml.Marshal(ToolDefsFile{Tools: defs})
			require.NoError(t, err)
			assert.NotContains(t, string(encoded), "legacytext")
			assert.NotContains(t, string(encoded), "items:")
			roundTrip, err := ParseToolDefs(encoded)
			require.NoError(t, err)
			require.Len(t, roundTrip, 1)
			assert.Equal(t, defs[0].SideEffects.LegacyText, roundTrip[0].SideEffects.LegacyText)
			assert.Len(t, roundTrip[0].SideEffects.Items, len(defs[0].SideEffects.Items))
			if len(defs[0].SideEffects.Items) > 0 {
				assert.Equal(t, defs[0].SideEffects.Items, roundTrip[0].SideEffects.Items)
			}
		})
	}
}

func TestToolDefYAMLRoundTripPreservesSemantics(t *testing.T) {
	t.Parallel()
	input, err := os.ReadFile(filepath.Join("testdata", "exectool_tools.yaml"))
	require.NoError(t, err)
	defs, err := ParseToolDefs(input)
	require.NoError(t, err)
	encoded, err := yaml.Marshal(ToolDefsFile{Tools: defs})
	require.NoError(t, err)
	roundTrip, err := ParseToolDefs(encoded)
	require.NoError(t, err)
	assert.Equal(t, defs, roundTrip)
}

func TestToolSideEffectsMarshalRejectsAmbiguousInternalState(t *testing.T) {
	t.Parallel()
	_, err := yaml.Marshal(ToolSideEffects{
		LegacyText: "legacy",
		Items:      []ToolSideEffect{{Kind: "filesystem_write"}},
	})
	require.ErrorContains(t, err, "both legacy text and structured items")
}

func FuzzToolSideEffectsLegacyScalarRoundTrip(f *testing.F) {
	f.Add("creates files")
	f.Add("unicode Δ and newline\ntext")
	f.Add("")
	f.Fuzz(func(t *testing.T, text string) {
		if !utf8.ValidString(text) {
			t.Skip()
		}
		def := ToolDef{Name: "sample", Binary: "echo"}
		def.SideEffects.LegacyText = text
		encoded, err := yaml.Marshal(ToolDefsFile{Tools: []ToolDef{def}})
		require.NoError(t, err)
		roundTrip, err := ParseToolDefs(encoded)
		require.NoError(t, err, "encoded YAML:\n%s", encoded)
		require.Len(t, roundTrip, 1)
		assert.Equal(t, text, roundTrip[0].SideEffects.LegacyText)
	})
}

func TestToolDef_ToToolSpec_IgnoresStructuredContractFields(t *testing.T) {
	t.Parallel()

	td := ToolDef{
		Name:        "parse_csv",
		Description: "Parse CSV.",
		Binary:      "csvtool",
		Problem:     "Need structured CSV rows.",
		Goals:       []string{"Return rows."},
		SideEffects: ToolSideEffects{
			Items: []ToolSideEffect{{Kind: "none", Description: "Read-only."}},
		},
		Reversibility: ToolReversibility{Classification: "reversible", Undo: "noop"},
	}

	spec := td.ToToolSpec()
	assert.Equal(t, "Parse CSV.", spec.Description)
	assert.NotContains(t, spec.Description, "Need structured CSV rows")
	assert.NotContains(t, spec.Description, "Read-only")
}

func TestToolDef_ToToolSpec_Internal(t *testing.T) {
	t.Parallel()

	td := ToolDef{
		Name:       "internal_thing",
		Binary:     "true",
		Visibility: "internal",
	}
	spec := td.ToToolSpec()
	assert.Equal(t, core.Internal, spec.Visibility)
}
