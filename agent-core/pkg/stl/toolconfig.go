// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"encoding/json"
	"fmt"
)

// DecodeToolConfig decodes a ToolDef's Config map into a typed struct.
// Uses JSON marshal/unmarshal for type coercion (YAML numbers, nested
// slices, etc. are handled by the JSON layer). Returns an error
// mentioning the tool name if decoding fails.
func DecodeToolConfig(def ToolDef, target interface{}) error {
	if def.Config == nil {
		return nil
	}
	data, err := json.Marshal(def.Config)
	if err != nil {
		return fmt.Errorf("tool %q config marshal: %w", def.Name, err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("tool %q config: %w", def.Name, err)
	}
	return nil
}

// ChildAgentConfig holds child agent invocation parameters read from
// a tool's config block. Used by launch_eval and execute_task.
type ChildAgentConfig struct {
	Machine          string   `json:"machine"`
	Tools            string   `json:"tools"`
	ToolDeclarations []string `json:"tools_declarations"`
}

// LLMToolConfig holds LLM-related settings from an invoke_llm tool's
// config block. Replaces ad-hoc map reads in extractLLMConfig.
type LLMToolConfig struct {
	Model        string `json:"model"`
	Provider     string `json:"provider"`
	ProviderURL  string `json:"provider_url"`
	OllamaURL    string `json:"ollama_url"`
	SystemPrompt string `json:"system_prompt"`
	ToolPrompt   string `json:"tool_prompt"`
	NumCtx       int    `json:"num_ctx"`
	LLMTimeout   int    `json:"llm_timeout"`
	MaxTime      int    `json:"max_time"`
	MaxTokens    int    `json:"max_tokens"`
}

// LoadSuiteConfig holds config for the load_suite tool.
type LoadSuiteConfig struct {
	Input     string `json:"input"`
	OutputDir string `json:"output_dir"`
	Reps      int    `json:"reps"`
	Timeout   int    `json:"timeout"`
	OllamaURL string `json:"ollama_url"`
}

// RunPointConfig holds config for the run_point tool.
type RunPointConfig struct {
	PointMachine string `json:"point_machine"`
}

// ServeUIToolConfig holds config for the serve_ui bench tool.
type ServeUIToolConfig struct {
	Addr        string `json:"addr"`
	DataDir     string `json:"data_dir"`
	ConfigsDir  string `json:"configs_dir"`
	DocsDir     string `json:"docs_dir"`
	SourceDir   string `json:"source_dir"`
	ProfilesDir string `json:"profiles_dir"`
}
