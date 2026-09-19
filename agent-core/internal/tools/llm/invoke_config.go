// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package llm

import (
	"fmt"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

// DecodeInvokeLLMConfig decodes and validates invoke_llm config.
func DecodeInvokeLLMConfig(def catalog.ToolDef) (catalog.LLMToolConfig, error) {
	var cfg catalog.LLMToolConfig
	if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
		return catalog.LLMToolConfig{}, err
	}
	if cfg.Dialect != "" && cfg.Provider != "" {
		return catalog.LLMToolConfig{}, fmt.Errorf(
			"invoke_llm config names both provider %q and dialect %q; a dialect replaces the provider", cfg.Provider, cfg.Dialect)
	}
	if cfg.Dialect == "" && cfg.Provider == "" {
		cfg.Provider = "ollama"
	}
	if cfg.ProviderURL == "" {
		cfg.ProviderURL = cfg.OllamaURL
	}
	if cfg.ProviderURL == "" && cfg.Provider == "ollama" {
		cfg.ProviderURL = "http://localhost:11434"
	}
	if cfg.Model == "" {
		return catalog.LLMToolConfig{}, fmt.Errorf("invoke_llm config requires model")
	}
	if cfg.ManifestState == "" {
		return catalog.LLMToolConfig{}, fmt.Errorf("invoke_llm config requires manifest_state")
	}
	return cfg, nil
}
