// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package llm

import (
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func LoadProfilesFromBytes(files map[string][]byte) (*ProfileRegistry, error) {
	return loadProfilesFromBytes(files)
}

func (r *ProfileRegistry) ResolveProfileSpec(model string) ProfileSpec {
	return r.resolveProfileSpec(model)
}

func (r *ProfileRegistry) ProfileNames() []string {
	return r.profileNames()
}

func TestDefaultProfile_ExtractToolCall(t *testing.T) {
	p := DefaultProfile()
	raw := `[tool_call]{"tool":"read","parameters":{"path":"f.go"}}[/tool_call]`
	result := p.ExtractToolCall(raw)
	assert.Equal(t, `{"tool":"read","parameters":{"path":"f.go"}}`, result)
}

func TestDefaultProfile_EnvelopeConfig(t *testing.T) {
	p := DefaultProfile()
	env, strict := p.EnvelopeConfig()
	require.NotNil(t, env)
	assert.Equal(t, "[tool_call]", env.Open)
	assert.Equal(t, "[/tool_call]", env.Close)
	assert.False(t, strict)
}

func TestStripCodeFences(t *testing.T) {
	input := "```json\n{\"tool\":\"read\"}\n```"
	assert.Equal(t, `{"tool":"read"}`, StripCodeFences(input))
}

func TestStripCodeFences_NoFences(t *testing.T) {
	input := `{"tool":"read"}`
	assert.Equal(t, input, StripCodeFences(input))
}

func TestStripThinkingBlocks(t *testing.T) {
	input := `<think>let me think about this</think>{"tool":"read"}`
	assert.Equal(t, `{"tool":"read"}`, StripThinkingBlocks(input))
}

func TestStripThinkingBlocks_ThinkingTag(t *testing.T) {
	input := `<thinking>reasoning here</thinking>{"tool":"write"}`
	assert.Equal(t, `{"tool":"write"}`, StripThinkingBlocks(input))
}

func TestStripThinkingBlocks_BothTags(t *testing.T) {
	input := `<think>first</think><thinking>second</thinking>{"tool":"edit"}`
	assert.Equal(t, `{"tool":"edit"}`, StripThinkingBlocks(input))
}

func TestStripThinkingBlocks_Unclosed(t *testing.T) {
	input := `<think>thinking forever`
	assert.Equal(t, "", StripThinkingBlocks(input))
}

func TestStripThinkingBlocks_UnclosedThinking(t *testing.T) {
	input := `<thinking>reasoning forever`
	assert.Equal(t, "", StripThinkingBlocks(input))
}

func TestExtractWithEnvelope(t *testing.T) {
	input := `Some text [tool_call]{"tool":"read"}[/tool_call] more text`
	result := ExtractWithEnvelope(input, "[tool_call]", "[/tool_call]")
	assert.Equal(t, `{"tool":"read"}`, result)
}

func TestExtractWithEnvelope_FallbackToBraces(t *testing.T) {
	input := `Some preamble {"tool":"read"} trailing`
	result := ExtractWithEnvelope(input, "[tool_call]", "[/tool_call]")
	assert.Equal(t, `{"tool":"read"}`, result)
}

func TestExtractBraces(t *testing.T) {
	assert.Equal(t, `{"tool":"read"}`, ExtractBraces(`prefix {"tool":"read"} suffix`))
	assert.Equal(t, `{"tool":"read"}`, ExtractBraces(`{"tool":"read"}`))
}

func TestMakeNativeTokenExtractor(t *testing.T) {
	extract := MakeNativeTokenExtractor("<|end|>")
	result := extract(`{"tool":"read"}<|end|>`)
	assert.Equal(t, `{"tool":"read"}`, result)
}

func TestMakeNativeTokenExtractor_NoToken(t *testing.T) {
	extract := MakeNativeTokenExtractor("<|end|>")
	input := `{"tool":"read"}`
	assert.Equal(t, input, extract(input))
}

func TestLoadProfilesFromBytes(t *testing.T) {
	files := map[string][]byte{
		"default.yaml": []byte(`
name: default
envelope:
  open: "[tool_call]"
  close: "[/tool_call]"
extraction_pipeline:
  - extract_envelope:
      open: "[tool_call]"
      close: "[/tool_call]"
`),
		"qwen.yaml": []byte(`
name: qwen
match_prefixes:
  - qwen
strict_format: true
extraction_pipeline:
  - strip_thinking_blocks
  - strip_code_fences
  - extract_braces
`),
	}

	reg, err := LoadProfilesFromBytes(files)
	require.NoError(t, err)

	names := reg.ProfileNames()
	assert.Contains(t, names, "default")
	assert.Contains(t, names, "qwen")

	defaultParser := reg.ResolveProfile("llama3:latest")
	env, _ := defaultParser.EnvelopeConfig()
	require.NotNil(t, env)
	assert.Equal(t, "[tool_call]", env.Open)

	qwenParser := reg.ResolveProfile("qwen3-coder:latest")
	env, strict := qwenParser.EnvelopeConfig()
	assert.Nil(t, env)
	assert.True(t, strict)
}

func TestLoadProfilesFromBytes_NoDefault(t *testing.T) {
	files := map[string][]byte{
		"qwen.yaml": []byte(`
name: qwen
match_prefixes:
  - qwen
extraction_pipeline:
  - extract_braces
`),
	}
	_, err := LoadProfilesFromBytes(files)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no default profile")
}

func TestDefaultProfileRegistry(t *testing.T) {
	reg, err := DefaultProfileRegistry()
	require.NoError(t, err)

	names := reg.ProfileNames()
	assert.Contains(t, names, "default")
	assert.Contains(t, names, "qwen")
	assert.Contains(t, names, "deepseek")
	assert.Contains(t, names, "gemma")

	// Default profile resolves for an unknown model.
	dp := reg.ResolveProfile("llama3:latest")
	env, strict := dp.EnvelopeConfig()
	require.NotNil(t, env)
	assert.Equal(t, "[tool_call]", env.Open)
	assert.False(t, strict)

	// Qwen prefix match.
	qp := reg.ResolveProfile("qwen3-coder:latest")
	_, qStrict := qp.EnvelopeConfig()
	assert.True(t, qStrict)

	// Deepseek prefix match.
	ds := reg.ResolveProfileSpec("deepseek-coder:latest")
	assert.Equal(t, "deepseek", ds.ProfileName)

	// Gemma uses <tool_call> envelope.
	gp := reg.ResolveProfile("gemma3:latest")
	gEnv, gStrict := gp.EnvelopeConfig()
	require.NotNil(t, gEnv)
	assert.Equal(t, "<tool_call>", gEnv.Open)
	assert.Equal(t, "</tool_call>", gEnv.Close)
	assert.True(t, gStrict)
}

func TestLoadProfilesFromFS_Empty(t *testing.T) {
	emptyFS := fstest.MapFS{}
	_, err := LoadProfilesFromFS(emptyFS)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no default profile")
}

func TestResolveProfileSpec(t *testing.T) {
	files := map[string][]byte{
		"default.yaml": []byte(`
name: default
envelope:
  open: "[tool_call]"
  close: "[/tool_call]"
extraction_pipeline:
  - extract_braces
`),
		"deepseek.yaml": []byte(`
name: deepseek
match_prefixes:
  - deepseek
extraction_pipeline:
  - extract_braces
`),
	}
	reg, err := LoadProfilesFromBytes(files)
	require.NoError(t, err)

	spec := reg.ResolveProfileSpec("deepseek-coder:latest")
	assert.Equal(t, "deepseek", spec.ProfileName)

	spec = reg.ResolveProfileSpec("llama3:latest")
	assert.Equal(t, "default", spec.ProfileName)
}

func TestResponseProfileRejectsMachineSelection(t *testing.T) {
	files := map[string][]byte{
		"default.yaml": []byte(`
name: default
machine: hidden-program
extraction_pipeline: [extract_braces]
`),
	}

	_, err := LoadProfilesFromBytes(files)

	require.ErrorContains(t, err, "machine is not supported")
	require.ErrorContains(t, err, "MachineSpec owns program selection")
}

func TestLoadProfilesFromBytes_ValidatesEveryPipelineOperation(t *testing.T) {
	tests := []struct {
		name string
		step string
	}{
		{name: "strip code fences", step: "strip_code_fences"},
		{name: "strip thinking blocks", step: "strip_thinking_blocks"},
		{name: "extract braces", step: "extract_braces"},
		{
			name: "extract envelope",
			step: "extract_envelope:\n      open: \"[tool_call]\"\n      close: \"[/tool_call]\"",
		},
		{
			name: "extract native token",
			step: "extract_native_token:\n      token: \"<|end|>\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadProfilesFromBytes(singleProfileWithStep(tt.step))
			require.NoError(t, err)
		})
	}
}

func TestLoadProfilesFromBytes_RejectsInvalidPipelineOperations(t *testing.T) {
	tests := []struct {
		name    string
		step    string
		wantErr string
	}{
		{
			name:    "strip code fences parameter",
			step:    "strip_code_fences:\n      unexpected: value",
			wantErr: `operation "strip_code_fences" does not allow parameter "unexpected"`,
		},
		{
			name:    "strip thinking blocks parameter",
			step:    "strip_thinking_blocks:\n      unexpected: value",
			wantErr: `operation "strip_thinking_blocks" does not allow parameter "unexpected"`,
		},
		{
			name:    "extract braces parameter",
			step:    "extract_braces:\n      unexpected: value",
			wantErr: `operation "extract_braces" does not allow parameter "unexpected"`,
		},
		{
			name:    "extract envelope missing close",
			step:    "extract_envelope:\n      open: \"[tool_call]\"",
			wantErr: `operation "extract_envelope" requires non-empty parameter "close"`,
		},
		{
			name: "extract envelope unexpected parameter",
			step: "extract_envelope:\n      open: \"[tool_call]\"\n      close: \"[/tool_call]\"" +
				"\n      unexpected: value",
			wantErr: `operation "extract_envelope" does not allow parameter "unexpected"`,
		},
		{
			name:    "extract native token missing token",
			step:    "extract_native_token: {}",
			wantErr: `operation "extract_native_token" requires non-empty parameter "token"`,
		},
		{
			name:    "extract native token unexpected parameter",
			step:    "extract_native_token:\n      token: \"<|end|>\"\n      unexpected: value",
			wantErr: `operation "extract_native_token" does not allow parameter "unexpected"`,
		},
		{
			name:    "unknown operation",
			step:    "extract_magic",
			wantErr: `unknown operation "extract_magic"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadProfilesFromBytes(singleProfileWithStep(tt.step))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "profile invalid.yaml: extraction_pipeline[0]")
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestLoadProfilesFromBytes_RejectsMalformedPipelineMaps(t *testing.T) {
	tests := []struct {
		name    string
		step    string
		wantErr string
	}{
		{
			name:    "multiple operations",
			step:    "{extract_braces: {}, strip_code_fences: {}}",
			wantErr: "pipeline step map must have exactly one key",
		},
		{
			name:    "non-map parameters",
			step:    "extract_native_token: \"<|end|>\"",
			wantErr: `pipeline step "extract_native_token" parameters must be a map`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadProfilesFromBytes(singleProfileWithStep(tt.step))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "parse profile invalid.yaml")
			assert.Contains(t, err.Error(), "extraction_pipeline[0]")
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestLoadProfilesFromBytes_ReportsPipelineStepIndex(t *testing.T) {
	files := map[string][]byte{
		"profile.yaml": []byte(
			"name: default\nextraction_pipeline:\n  - extract_braces\n  - extract_magic\n",
		),
	}

	_, err := LoadProfilesFromBytes(files)
	require.EqualError(
		t,
		err,
		`profile profile.yaml: extraction_pipeline[1]: unknown operation "extract_magic"`,
	)
}

func TestLoadProfilesFromBytes_RejectsRegistryAmbiguity(t *testing.T) {
	tests := []struct {
		name    string
		files   map[string][]byte
		wantErr string
	}{
		{
			name: "duplicate name",
			files: profileFileSet(
				"profile-a.yaml", "qwen", "qwen",
				"profile-b.yaml", "qwen", "other",
			),
			wantErr: `profile profile-b.yaml: duplicate name "qwen" (already declared in profile-a.yaml)`,
		},
		{
			name: "duplicate default",
			files: map[string][]byte{
				"fallback-a.yaml": []byte("name: fallback-a\nextraction_pipeline:\n  - extract_braces\n"),
				"fallback-b.yaml": []byte("name: fallback-b\nextraction_pipeline:\n  - extract_braces\n"),
			},
			wantErr: "profile fallback-b.yaml: duplicate default profile (already declared in fallback-a.yaml)",
		},
		{
			name: "overlapping prefixes",
			files: profileFileSet(
				"profile-a.yaml", "qwen", "qwen",
				"profile-b.yaml", "qwen3", "qwen3",
			),
			wantErr: `profile profile-b.yaml: match prefix "qwen3" is ambiguous with "qwen" from profile "qwen" in profile-a.yaml`,
		},
		{
			name: "case-insensitive prefixes",
			files: profileFileSet(
				"profile-a.yaml", "qwen", "QWEN",
				"profile-b.yaml", "other", "qwen",
			),
			wantErr: `profile profile-b.yaml: match prefix "qwen" is ambiguous with "qwen" from profile "qwen" in profile-a.yaml`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadProfilesFromBytes(tt.files)
			require.EqualError(t, err, tt.wantErr)
		})
	}
}

func TestLoadProfilesFromBytes_RejectsEmptyPrefix(t *testing.T) {
	files := map[string][]byte{
		"00-default.yaml": []byte("name: default\nextraction_pipeline:\n  - extract_braces\n"),
		"profile.yaml": []byte(
			"name: qwen\nmatch_prefixes: [\" \"]\nextraction_pipeline:\n  - extract_braces\n",
		),
	}

	_, err := LoadProfilesFromBytes(files)
	require.EqualError(t, err, "profile profile.yaml: match_prefixes[0] must not be empty")
}

func TestLoadProfilesFromBytes_IsDeterministic(t *testing.T) {
	files := map[string][]byte{
		"z-default.yaml":  []byte("name: default\nextraction_pipeline:\n  - extract_braces\n"),
		"b-qwen.yaml":     profileYAML("qwen", "qwen"),
		"a-deepseek.yaml": profileYAML("deepseek", "deepseek"),
	}

	for range 100 {
		reg, err := LoadProfilesFromBytes(files)
		require.NoError(t, err)
		assert.Equal(t, []string{"default", "deepseek", "qwen"}, reg.ProfileNames())
		assert.Equal(t, "qwen", reg.ResolveProfileSpec("qwen3:latest").ProfileName)
		assert.Equal(t, "deepseek", reg.ResolveProfileSpec("deepseek-r1").ProfileName)
	}
}

func TestLoadProfilesFromBytes_HasDeterministicParameterDiagnostics(t *testing.T) {
	files := singleProfileWithStep("extract_braces:\n      zeta: value\n      alpha: value")

	for range 100 {
		_, err := LoadProfilesFromBytes(files)
		require.EqualError(
			t,
			err,
			`profile invalid.yaml: extraction_pipeline[0]: operation "extract_braces" does not allow parameter "alpha"`,
		)
	}
}

func singleProfileWithStep(step string) map[string][]byte {
	return map[string][]byte{
		"invalid.yaml": []byte("name: default\nextraction_pipeline:\n  - " + step + "\n"),
	}
}

func profileFileSet(
	firstFile, firstName, firstPrefix string,
	secondFile, secondName, secondPrefix string,
) map[string][]byte {
	return map[string][]byte{
		"00-default.yaml": []byte("name: default\nextraction_pipeline:\n  - extract_braces\n"),
		firstFile:         profileYAML(firstName, firstPrefix),
		secondFile:        profileYAML(secondName, secondPrefix),
	}
}

func profileYAML(name, prefix string) []byte {
	return []byte(
		"name: " + name + "\nmatch_prefixes:\n  - " + prefix +
			"\nextraction_pipeline:\n  - extract_braces\n",
	)
}
