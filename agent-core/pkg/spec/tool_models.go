// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package spec

import (
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

// ToolSelection is a parsed agents/*/tools.yaml file listing the tool
// names selected for a particular agent mode.
type ToolSelection struct {
	Tools []string `yaml:"tools"`
}

// ToolDeclaration captures tool contract fields needed for public spec-corpus
// validation. It is a stable public view of the runtime catalog model.
type ToolDeclaration struct {
	Name          string
	Type          string
	Category      string
	Contract      string
	Init          string
	Problem       string
	Goals         []string
	Requirements  ToolDeclRequirements
	NonGoals      []string
	Emits         []string
	Signature     *catalog.ToolSignature
	Output        ToolDeclOutput
	Metrics       core.MetricConfig
	Visibility    string
	Reversibility ToolDeclReversibility
	Undo          ToolDeclUndo
	SideEffects   ToolDeclSideEffects
	Errors        []ToolDeclError
	Relationships ToolDeclRelationships
	SourceFile    string
}

// ToolDeclRequirements captures observable behavior requirements used by the
// audit without importing the runtime STL package.
type ToolDeclRequirements struct {
	Input  []string
	Output []string
	Errors []string
}

// ToolDeclOutput captures the declared machine-readable result shape.
type ToolDeclOutput struct {
	Schema map[string]any
}

// ToolDeclReversibility captures the reversibility classification.
type ToolDeclReversibility struct {
	Classification string
}

// ToolDeclUndo captures the undo contract.
type ToolDeclUndo struct {
	Strategy string
	Payload  string
	Captures []string
}

// ToolDeclSideEffects handles both structured and legacy side_effects.
type ToolDeclSideEffects struct {
	LegacyText string
	Items      []ToolDeclSideEffect
}

// ToolDeclSideEffect captures one structured side-effect entry.
type ToolDeclSideEffect struct {
	Kind   string
	Target string
}

// ToolDeclError captures a declared failure mode.
type ToolDeclError struct {
	Signal string
}

// ToolDeclRelationships captures sequencing and overlap documentation.
type ToolDeclRelationships struct {
	Before   []string
	After    []string
	Overlaps []ToolDeclRelationshipRef
}

// ToolDeclRelationshipRef captures one related tool reference and how the
// declaring tool differs from it.
type ToolDeclRelationshipRef struct {
	Tool       string
	Difference string
}

func toolDeclarationFromDef(def catalog.ToolDef) ToolDeclaration {
	source := def.DeclarationSource()
	return ToolDeclaration{
		Name: def.Name, Type: def.Type, Category: def.Category, Contract: def.Contract,
		Init: def.Init, Problem: def.Problem, Goals: def.Goals, NonGoals: def.NonGoals,
		Emits: def.Emits, Metrics: def.Metrics, Visibility: def.Visibility,
		Signature: def.Signature,
		Requirements: ToolDeclRequirements{
			Input: def.Requirements.Input, Output: def.Requirements.Output, Errors: def.Requirements.Errors,
		},
		Output:        ToolDeclOutput{Schema: def.Output.Schema},
		Reversibility: ToolDeclReversibility{Classification: def.Reversibility.Classification},
		Undo: ToolDeclUndo{
			Strategy: def.Undo.Strategy, Payload: def.Undo.Payload, Captures: def.Undo.Captures,
		},
		SideEffects: ToolDeclSideEffects{
			LegacyText: def.SideEffects.LegacyText,
			Items:      toolSideEffectsFromDefs(def.SideEffects.Items),
		},
		Errors:        toolErrorsFromDefs(def.Errors),
		Relationships: toolRelationshipsFromDef(def.Relationships),
		SourceFile:    source.Path,
	}
}

func toolSideEffectsFromDefs(items []catalog.ToolSideEffect) []ToolDeclSideEffect {
	result := make([]ToolDeclSideEffect, len(items))
	for index, item := range items {
		result[index] = ToolDeclSideEffect{Kind: item.Kind, Target: item.Target}
	}
	return result
}

func toolErrorsFromDefs(items []catalog.ToolErrorContract) []ToolDeclError {
	result := make([]ToolDeclError, len(items))
	for index, item := range items {
		result[index] = ToolDeclError{Signal: item.Signal}
	}
	return result
}

func toolRelationshipsFromDef(value catalog.ToolRelationships) ToolDeclRelationships {
	overlaps := make([]ToolDeclRelationshipRef, len(value.Overlaps))
	for index, overlap := range value.Overlaps {
		overlaps[index] = ToolDeclRelationshipRef{Tool: overlap.Tool, Difference: overlap.Difference}
	}
	return ToolDeclRelationships{Before: value.Before, After: value.After, Overlaps: overlaps}
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
