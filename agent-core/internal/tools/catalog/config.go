// Copyright (c) 2026 Nokia. All rights reserved.

package catalog

import (
	"encoding/json"
	"fmt"
)

const defaultCheckpointSelector = "latest"

// DecodeToolConfig decodes a ToolDef's Config map into a typed struct.
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

// ChildAgentConfig holds child agent invocation parameters.
type ChildAgentConfig struct {
	Profile          string   `json:"profile"`
	Machine          string   `json:"machine"`
	Tools            string   `json:"tools"`
	ToolDeclarations []string `json:"tools_declarations"`
}

// CheckpointHistoryConfig holds config for checkpoint_history.
type CheckpointHistoryConfig struct {
	Checkpoint string `json:"checkpoint"`
	Input      string `json:"input"`
}

// SelectedCheckpoint returns the configured checkpoint ID or latest.
func (c CheckpointHistoryConfig) SelectedCheckpoint() string {
	if c.Checkpoint == "" {
		return defaultCheckpointSelector
	}
	return c.Checkpoint
}

// CheckpointRollbackConfig holds config for checkpoint_rollback.
type CheckpointRollbackConfig struct {
	Checkpoint       string `json:"checkpoint"`
	ToIteration      *int   `json:"to_iteration"`
	Input            string `json:"input"`
	Directory        string `json:"directory"`
	RestoreWorkspace bool   `json:"restore_workspace"`
}

// SelectedCheckpoint returns the configured checkpoint ID or latest.
func (c CheckpointRollbackConfig) SelectedCheckpoint() string {
	if c.Checkpoint == "" {
		return defaultCheckpointSelector
	}
	return c.Checkpoint
}

// HasTargetIteration reports whether rollback received an explicit target.
func (c CheckpointRollbackConfig) HasTargetIteration() bool {
	return c.ToIteration != nil
}

// LLMToolConfig holds model-boundary settings from invoke_llm config.
type LLMToolConfig struct {
	Model         string `json:"model"`
	Provider      string `json:"provider"`
	ProviderURL   string `json:"provider_url"`
	OllamaURL     string `json:"ollama_url"`
	ManifestState string `json:"manifest_state"`
	SystemPrompt  string `json:"system_prompt"`
	ToolPrompt    string `json:"tool_prompt"`
	NumCtx        int    `json:"num_ctx"`
	LLMTimeout    int    `json:"llm_timeout"`
	MaxTime       int    `json:"max_time"`
	MaxTokens     int    `json:"max_tokens"`
}

// LoadSuiteConfig holds config for evaluator session setup tools.
type LoadSuiteConfig struct {
	Input     string `json:"input"`
	OutputDir string `json:"output_dir"`
	Reps      int    `json:"reps"`
	Timeout   int    `json:"timeout"`
	OllamaURL string `json:"ollama_url"`
}

// RunPointConfig holds config for the run_point tool.
type RunPointConfig struct {
	PointMachine  string `json:"point_machine"`
	PointTools    string `json:"point_tools"`
	AgentName     string `json:"agent_name"`
	MaxIterations int    `json:"max_iterations"`
	SuccessState  string `json:"success_state"`
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

// ValidateChildAgentConfig checks fields required to invoke a child agent.
func ValidateChildAgentConfig(toolName string, cfg ChildAgentConfig) error {
	if cfg.Profile != "" {
		return nil
	}
	if cfg.Machine == "" {
		return fmt.Errorf("tool %q config requires profile or legacy machine", toolName)
	}
	if cfg.Tools == "" {
		return fmt.Errorf("tool %q config requires tools", toolName)
	}
	if len(cfg.ToolDeclarations) == 0 {
		return fmt.Errorf("tool %q config requires tools_declarations", toolName)
	}
	return nil
}

// ValidateRunPointConfig checks fields required to run a nested point machine.
func ValidateRunPointConfig(toolName string, cfg RunPointConfig) error {
	if cfg.PointMachine == "" {
		return fmt.Errorf("tool %q config requires point_machine", toolName)
	}
	if cfg.PointTools == "" {
		return fmt.Errorf("tool %q config requires point_tools", toolName)
	}
	return nil
}
