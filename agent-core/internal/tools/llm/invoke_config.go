// Copyright (c) 2026 Nokia. All rights reserved.

package llm

import (
	"fmt"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

// DecodeInvokeLLMConfig decodes and validates invoke_llm config.
func DecodeInvokeLLMConfig(def catalog.ToolDef) (catalog.LLMToolConfig, error) {
	cfg := catalog.LLMToolConfig{
		Provider:      "ollama",
		ProviderURL:   "http://localhost:11434",
		ManifestState: "Composing",
	}
	if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
		return catalog.LLMToolConfig{}, err
	}
	if cfg.ProviderURL == "" {
		cfg.ProviderURL = cfg.OllamaURL
	}
	if cfg.Model == "" {
		return catalog.LLMToolConfig{}, fmt.Errorf("invoke_llm config requires model")
	}
	if cfg.ManifestState == "" {
		return catalog.LLMToolConfig{}, fmt.Errorf("invoke_llm config requires manifest_state")
	}
	return cfg, nil
}
