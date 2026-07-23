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
	Profile string `json:"profile"`
}

// ComposeConfig holds the compose word's template and its $from(label).path input
// selectors, resolved against the command-state view (srd038).
type ComposeConfig struct {
	Template string            `json:"template"`
	Inputs   map[string]string `json:"inputs"`
	Signal   string            `json:"signal"`
}

// CompareConfig holds the compare_state word's two $from(label).path operand
// selectors and the signals it emits for an equal and an unequal verdict,
// resolved against the command-state view (srd038).
type CompareConfig struct {
	Left     string `json:"left"`
	Right    string `json:"right"`
	Matched  string `json:"matched"`
	Differed string `json:"differed"`
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
	Model           string `json:"model"`
	Provider        string `json:"provider"`
	ProviderURL     string `json:"provider_url"`
	OllamaURL       string `json:"ollama_url"`
	ManifestState   string `json:"manifest_state"`
	ResponseProfile string `json:"response_profile"`
	SystemPrompt    string `json:"system_prompt"`
	ToolPrompt      string `json:"tool_prompt"`
	// UserPromptFrom optionally sources the user message from a command-state
	// $from(label).path selector instead of the previous Result's Output, so a
	// word dispatched non-adjacently (for example a chat-LLM word reached through
	// a $tool router) can read a non-adjacent composed prompt. Omitted: the user
	// message stays the previous Result's Output.
	UserPromptFrom string `json:"user_prompt_from"`
	// AnswerOnly omits the tool manifest from the prompt so the word produces a
	// final answer rather than a tool call. Set for a chat-LLM word a $tool router
	// dispatches, which the manifest of the state it runs in would otherwise offer
	// the chat-LLM vocabulary (including itself).
	AnswerOnly bool `json:"answer_only"`
	NumCtx     int  `json:"num_ctx"`
	LLMTimeout int  `json:"llm_timeout"`
	MaxTime    int  `json:"max_time"`
	MaxTokens  int  `json:"max_tokens"`
	// Temperature and Seed are optional decoding parameters. Pointers so an
	// omitted field is distinguishable from an explicit zero: nil selects the
	// deterministic defaults (temperature 0, seed 42) applied at build time.
	Temperature *float64 `json:"temperature"`
	Seed        *int     `json:"seed"`
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
	if cfg.Profile == "" {
		return fmt.Errorf("tool %q config requires profile", toolName)
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
