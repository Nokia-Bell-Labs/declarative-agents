// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// LLMConfig is a YAML-driven configuration for an LLM tool instance.
// Each agent directory can define one or more LLM configs (e.g.
// configs/generator/llm/default.yaml, configs/generator/llm/deepseek.yaml).
//
// The Name field becomes the tool name in the registry (default: invoke_llm).
// Multiple LLM tools can coexist by using different names.
type LLMConfig struct {
	Name     string `yaml:"name"`
	Model    string `yaml:"model"`
	Provider string `yaml:"provider"`

	// Provider connection
	ProviderURL string `yaml:"provider_url,omitempty"`

	// Prompt sections — assembled into the system message
	SystemPrompt string `yaml:"system_prompt"`
	ToolPrompt   string `yaml:"tool_prompt,omitempty"`

	// Resource limits
	NumCtx     int `yaml:"num_ctx,omitempty"`
	LLMTimeout int `yaml:"llm_timeout,omitempty"`
	MaxTime    int `yaml:"max_time,omitempty"`
	MaxTokens  int `yaml:"max_tokens,omitempty"`
}

// EffectiveName returns the tool name, defaulting to "invoke_llm".
func (c LLMConfig) EffectiveName() string {
	if c.Name == "" {
		return "invoke_llm"
	}
	return c.Name
}

// EffectiveProviderURL returns the provider URL, with a default
// based on the provider type.
func (c LLMConfig) EffectiveProviderURL() string {
	if c.ProviderURL != "" {
		return c.ProviderURL
	}
	switch c.Provider {
	case "ollama":
		return "http://localhost:11434"
	default:
		return ""
	}
}

// LLMTimeoutDuration returns the per-call timeout as a Duration.
func (c LLMConfig) LLMTimeoutDuration() time.Duration {
	if c.LLMTimeout > 0 {
		return time.Duration(c.LLMTimeout) * time.Second
	}
	return 0
}

// MaxTimeDuration returns the max wall-clock time as a Duration.
func (c LLMConfig) MaxTimeDuration() time.Duration {
	if c.MaxTime > 0 {
		return time.Duration(c.MaxTime) * time.Second
	}
	return 0
}

// LoadLLMConfig reads an LLM configuration YAML file.
func LoadLLMConfig(path string) (LLMConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return LLMConfig{}, fmt.Errorf("read llm config %s: %w", path, err)
	}
	return ParseLLMConfig(data, path)
}

// ParseLLMConfig parses LLM configuration from YAML bytes.
func ParseLLMConfig(data []byte, source string) (LLMConfig, error) {
	var cfg LLMConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return LLMConfig{}, fmt.Errorf("parse llm config %s: %w", source, err)
	}
	if cfg.Model == "" {
		return LLMConfig{}, fmt.Errorf("llm config %s: model is required", source)
	}
	if cfg.Provider == "" {
		return LLMConfig{}, fmt.Errorf("llm config %s: provider is required", source)
	}
	return cfg, nil
}

// LoadLLMConfigs loads multiple LLM configuration files.
func LoadLLMConfigs(paths []string) ([]LLMConfig, error) {
	var configs []LLMConfig
	for _, p := range paths {
		cfg, err := LoadLLMConfig(p)
		if err != nil {
			return nil, err
		}
		configs = append(configs, cfg)
	}
	return configs, nil
}
