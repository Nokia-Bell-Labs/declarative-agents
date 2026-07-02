// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import (
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
	"gopkg.in/yaml.v3"
)

// ToolSelection is a parsed agents/*/tools.yaml file listing the tool
// names selected for a particular agent mode.
type ToolSelection struct {
	Tools []string `yaml:"tools"`
}

// ToolDeclaration captures tool contract fields needed for public spec-corpus
// validation. It mirrors the runtime ToolDef fields owned by
// internal/tools/catalog without importing an internal package.
type ToolDeclaration struct {
	Name          string                `yaml:"name"`
	Type          string                `yaml:"type,omitempty"`
	Category      string                `yaml:"category,omitempty"`
	Contract      string                `yaml:"contract,omitempty"`
	Init          string                `yaml:"init,omitempty"`
	Problem       string                `yaml:"problem,omitempty"`
	Goals         []string              `yaml:"goals,omitempty"`
	Requirements  ToolDeclRequirements  `yaml:"requirements,omitempty"`
	NonGoals      []string              `yaml:"non_goals,omitempty"`
	Emits         []string              `yaml:"emits,omitempty"`
	Output        ToolDeclOutput        `yaml:"output,omitempty"`
	Metrics       core.MetricConfig     `yaml:"metrics,omitempty"`
	Visibility    string                `yaml:"visibility,omitempty"`
	Reversibility ToolDeclReversibility `yaml:"reversibility,omitempty"`
	Undo          ToolDeclUndo          `yaml:"undo,omitempty"`
	SideEffects   ToolDeclSideEffects   `yaml:"side_effects,omitempty"`
	Errors        []ToolDeclError       `yaml:"errors,omitempty"`
	Relationships ToolDeclRelationships `yaml:"relationships,omitempty"`
	SourceFile    string                `yaml:"-"`
}

// ToolDeclRequirements captures observable behavior requirements used by the
// audit without importing the runtime STL package.
type ToolDeclRequirements struct {
	Input  []string `yaml:"input,omitempty"`
	Output []string `yaml:"output,omitempty"`
	Errors []string `yaml:"errors,omitempty"`
}

// ToolDeclOutput captures the declared machine-readable result shape.
type ToolDeclOutput struct {
	Schema map[string]any `yaml:"schema,omitempty"`
}

// ToolDeclReversibility captures the reversibility classification.
type ToolDeclReversibility struct {
	Classification string `yaml:"classification,omitempty"`
}

// ToolDeclUndo captures the undo contract.
type ToolDeclUndo struct {
	Strategy string   `yaml:"strategy,omitempty"`
	Payload  string   `yaml:"payload,omitempty"`
	Captures []string `yaml:"captures,omitempty"`
}

// ToolDeclSideEffects handles both structured and legacy side_effects.
type ToolDeclSideEffects struct {
	Items []ToolDeclSideEffect
}

// ToolDeclSideEffect captures one structured side-effect entry.
type ToolDeclSideEffect struct {
	Kind string `yaml:"kind"`
}

// ToolDeclError captures a declared failure mode.
type ToolDeclError struct {
	Signal string `yaml:"signal,omitempty"`
}

// ToolDeclRelationships captures sequencing and overlap documentation.
type ToolDeclRelationships struct {
	Before   []string                  `yaml:"before,omitempty"`
	After    []string                  `yaml:"after,omitempty"`
	Overlaps []ToolDeclRelationshipRef `yaml:"overlaps,omitempty"`
}

// ToolDeclRelationshipRef captures one related tool reference.
type ToolDeclRelationshipRef struct {
	Tool string `yaml:"tool,omitempty"`
}

func (s *ToolDeclSideEffects) UnmarshalYAML(value *yaml.Node) error {
	var items []ToolDeclSideEffect
	if err := value.Decode(&items); err == nil {
		s.Items = items
		return nil
	}
	return nil
}

// ToolDeclFile is the top-level YAML structure for a tool declaration file.
type ToolDeclFile struct {
	Tools []ToolDeclaration `yaml:"tools"`
}

// KnownSideEffectKinds is the canonical vocabulary for side_effects kind values.
var KnownSideEffectKinds = map[string]bool{
	"filesystem_read":           true,
	"filesystem_write":          true,
	"command_state":             true,
	"state_mutation":            true,
	"state_read":                true,
	"child_tool_execution":      true,
	"child_agent_execution":     true,
	"child_process":             true,
	"nested_machine_execution":  true,
	"external_api":              true,
	"external_api_call":         true,
	"network_listen":            true,
	"network_listener_shutdown": true,
	"human_boundary":            true,
	"stderr_write":              true,
	"none":                      true,
}
