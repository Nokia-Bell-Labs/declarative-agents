// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadLLMConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "llm.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
name: invoke_llm
model: "qwen3.6:35b-mlx"
provider: ollama
provider_url: "http://localhost:11434"
system_prompt: |
  You are a coding assistant.
tool_prompt: |
  Return JSON tool calls.
max_time: 600
num_ctx: 8192
llm_timeout: 120
max_tokens: 4096
`), 0644))

	cfg, err := LoadLLMConfig(path)
	require.NoError(t, err)

	assert.Equal(t, "invoke_llm", cfg.Name)
	assert.Equal(t, "qwen3.6:35b-mlx", cfg.Model)
	assert.Equal(t, "ollama", cfg.Provider)
	assert.Equal(t, "http://localhost:11434", cfg.ProviderURL)
	assert.Contains(t, cfg.SystemPrompt, "coding assistant")
	assert.Contains(t, cfg.ToolPrompt, "JSON tool calls")
	assert.Equal(t, 600, cfg.MaxTime)
	assert.Equal(t, 8192, cfg.NumCtx)
	assert.Equal(t, 120, cfg.LLMTimeout)
	assert.Equal(t, 4096, cfg.MaxTokens)
}

func TestLLMConfig_EffectiveName(t *testing.T) {
	assert.Equal(t, "invoke_llm", LLMConfig{}.EffectiveName())
	assert.Equal(t, "custom_llm", LLMConfig{Name: "custom_llm"}.EffectiveName())
}

func TestLLMConfig_EffectiveProviderURL(t *testing.T) {
	assert.Equal(t, "http://localhost:11434", LLMConfig{Provider: "ollama"}.EffectiveProviderURL())
	assert.Equal(t, "http://custom:8080", LLMConfig{Provider: "ollama", ProviderURL: "http://custom:8080"}.EffectiveProviderURL())
	assert.Equal(t, "", LLMConfig{Provider: "unknown"}.EffectiveProviderURL())
}

func TestLLMConfig_Durations(t *testing.T) {
	cfg := LLMConfig{LLMTimeout: 120, MaxTime: 600}
	assert.Equal(t, 120*time.Second, cfg.LLMTimeoutDuration())
	assert.Equal(t, 600*time.Second, cfg.MaxTimeDuration())

	zero := LLMConfig{}
	assert.Equal(t, time.Duration(0), zero.LLMTimeoutDuration())
	assert.Equal(t, time.Duration(0), zero.MaxTimeDuration())
}

func TestLoadLLMConfig_MissingModel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "llm.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
provider: ollama
`), 0644))

	_, err := LoadLLMConfig(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "model is required")
}

func TestLoadLLMConfig_MissingProvider(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "llm.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
model: "test-model"
`), 0644))

	_, err := LoadLLMConfig(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider is required")
}

func TestLoadLLMConfigs_Multiple(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.yaml"), `
name: invoke_llm
model: model-a
provider: ollama
`)
	writeFile(t, filepath.Join(dir, "b.yaml"), `
name: invoke_llm_alt
model: model-b
provider: ollama
`)

	configs, err := LoadLLMConfigs([]string{
		filepath.Join(dir, "a.yaml"),
		filepath.Join(dir, "b.yaml"),
	})
	require.NoError(t, err)
	require.Len(t, configs, 2)
	assert.Equal(t, "invoke_llm", configs[0].Name)
	assert.Equal(t, "model-a", configs[0].Model)
	assert.Equal(t, "invoke_llm_alt", configs[1].Name)
	assert.Equal(t, "model-b", configs[1].Model)
}
