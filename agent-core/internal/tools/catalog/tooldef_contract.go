// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package catalog

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/yamlstrict"
)

// The contract a tool declaration states beside its parameters: its
// requirements, signature, side effects, reversibility, undo, errors, and
// relationships to neighboring tools.

// ToolRequirements groups observable behaviors a tool must satisfy.
type ToolRequirements struct {
	Input       []string `yaml:"input,omitempty"`
	Output      []string `yaml:"output,omitempty"`
	SideEffects []string `yaml:"side_effects,omitempty"`
	Undo        []string `yaml:"undo,omitempty"`
	Errors      []string `yaml:"errors,omitempty"`
}

// ToolSignature states the checked part of a tool's contract: the type it
// consumes, the type it returns, and the signals it can emit. Its presence
// defaults the prose contract blocks by category (srd051 R6).
type ToolSignature struct {
	Input  string   `yaml:"input,omitempty" json:"input,omitempty"`
	Output string   `yaml:"output,omitempty" json:"output,omitempty"`
	Emits  []string `yaml:"emits,omitempty" json:"emits,omitempty"`
}

// ToolOutputContract describes the structured output shape produced by a tool.
type ToolOutputContract struct {
	Description string                 `yaml:"description,omitempty"`
	Schema      map[string]interface{} `yaml:"schema,omitempty"`
	Mode        string                 `yaml:"mode,omitempty"`
}

// ToolSideEffect describes one world mutation performed by a tool.
type ToolSideEffect struct {
	Kind        string   `yaml:"kind,omitempty"`
	Target      string   `yaml:"target,omitempty"`
	Paths       []string `yaml:"paths,omitempty"`
	State       string   `yaml:"state,omitempty"`
	Description string   `yaml:"description,omitempty"`
}

// ToolSideEffects accepts legacy scalar and structured list side_effects.
type ToolSideEffects struct {
	LegacyText string
	Items      []ToolSideEffect
}

func (se *ToolSideEffects) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		var text string
		if err := value.Decode(&text); err != nil {
			return err
		}
		se.LegacyText = text
		return nil
	case yaml.SequenceNode:
		for _, item := range value.Content {
			if err := yamlstrict.CheckKnownFields(item, ToolSideEffect{}); err != nil {
				return fmt.Errorf("side_effects: %w", err)
			}
		}
		var items []ToolSideEffect
		if err := value.Decode(&items); err != nil {
			return err
		}
		se.Items = items
		return nil
	case 0:
		return nil
	default:
		return fmt.Errorf("side_effects must be a string or list")
	}
}

// MarshalYAML preserves the accepted scalar/list surface instead of exposing
// ToolSideEffects' internal representation.
func (se ToolSideEffects) MarshalYAML() (interface{}, error) {
	if se.LegacyText != "" && len(se.Items) > 0 {
		return nil, fmt.Errorf("side_effects cannot contain both legacy text and structured items")
	}
	if se.LegacyText != "" {
		return &yaml.Node{
			Kind:  yaml.ScalarNode,
			Tag:   "!!str",
			Value: se.LegacyText,
			Style: yaml.DoubleQuotedStyle,
		}, nil
	}
	if se.Items != nil {
		return se.Items, nil
	}
	return nil, nil
}

// IsZero lets yaml omit empty side_effects when serializing.
func (se ToolSideEffects) IsZero() bool {
	return se.LegacyText == "" && len(se.Items) == 0
}

// ToolReversibility classifies whether a tool's effects can be undone.
type ToolReversibility struct {
	Classification       string `yaml:"classification,omitempty"`
	RequiresConfirmation bool   `yaml:"requires_confirmation,omitempty"`
}

func (tr *ToolReversibility) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("reversibility must be a mapping")
	}
	for i := 0; i < len(value.Content); i += 2 {
		switch value.Content[i].Value {
		case "classification", "receipt", "requires_confirmation":
		default:
			return fmt.Errorf("reversibility has unknown field %q", value.Content[i].Value)
		}
	}
	type wire ToolReversibility
	var decoded wire
	if err := value.Decode(&decoded); err != nil {
		return err
	}
	*tr = ToolReversibility(decoded)
	return nil
}

// ToolUndoContract describes how to reverse or compensate tool effects.
type ToolUndoContract struct {
	Strategy    string   `yaml:"strategy,omitempty"`
	Description string   `yaml:"description,omitempty"`
	Payload     string   `yaml:"payload,omitempty"`
	Captures    []string `yaml:"captures,omitempty"`
	Requires    []string `yaml:"requires,omitempty"`
}

// ToolErrorContract describes one expected failure mode.
type ToolErrorContract struct {
	Signal            string `yaml:"signal,omitempty"`
	Condition         string `yaml:"condition,omitempty"`
	MessageShape      string `yaml:"message_shape,omitempty"`
	StateAfterFailure string `yaml:"state_after_failure,omitempty"`
}

// ToolRelationships documents common composition neighbors.
type ToolRelationships struct {
	Before   []string      `yaml:"before,omitempty"`
	After    []string      `yaml:"after,omitempty"`
	Overlaps []ToolOverlap `yaml:"overlaps,omitempty"`
}

// ToolOverlap explains how this tool differs from a similar tool.
type ToolOverlap struct {
	Tool       string `yaml:"tool,omitempty"`
	Difference string `yaml:"difference,omitempty"`
}
