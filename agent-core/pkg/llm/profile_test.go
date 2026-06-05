// Copyright (c) 2026 Nokia. All rights reserved.

package llm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestStripThinkingBlocks_Unclosed(t *testing.T) {
	input := `<think>thinking forever`
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

func TestProfileSpec_Machine(t *testing.T) {
	spec := ProfileSpec{
		ProfileName: "deepseek",
		MachineName: "deepseek-machine",
	}
	p := newYAMLProfile(spec)
	assert.Equal(t, "deepseek", p.Name())
	assert.Equal(t, "deepseek-machine", p.Machine())
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
machine: deepseek-machine
extraction_pipeline:
  - extract_braces
`),
	}
	reg, err := LoadProfilesFromBytes(files)
	require.NoError(t, err)

	spec := reg.ResolveProfileSpec("deepseek-coder:latest")
	assert.Equal(t, "deepseek", spec.ProfileName)
	assert.Equal(t, "deepseek-machine", spec.MachineName)

	spec = reg.ResolveProfileSpec("llama3:latest")
	assert.Equal(t, "default", spec.ProfileName)
}
