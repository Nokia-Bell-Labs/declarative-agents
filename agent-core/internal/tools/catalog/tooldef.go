// Copyright (c) 2026 Nokia. All rights reserved.

package catalog

import (
	"encoding/json"
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

var cliExtensionKeys = map[string]bool{
	"flag":       true,
	"positional": true,
	"bool_flag":  true,
	"default":    true,
	"position":   true,
}

// ToolDef is a declarative, YAML-driven tool definition.
type ToolDef struct {
	Name          string                 `yaml:"name"`
	Type          string                 `yaml:"type,omitempty"`
	Category      string                 `yaml:"category,omitempty"`
	Contract      string                 `yaml:"contract,omitempty"`
	Description   string                 `yaml:"description"`
	Problem       string                 `yaml:"problem,omitempty"`
	Goals         []string               `yaml:"goals,omitempty"`
	Requirements  ToolRequirements       `yaml:"requirements,omitempty"`
	NonGoals      []string               `yaml:"non_goals,omitempty"`
	Output        ToolOutputContract     `yaml:"output,omitempty"`
	Metrics       core.MetricConfig      `yaml:"metrics,omitempty"`
	SideEffects   ToolSideEffects        `yaml:"side_effects,omitempty"`
	Reversibility ToolReversibility      `yaml:"reversibility,omitempty"`
	Undo          ToolUndoContract       `yaml:"undo,omitempty"`
	Errors        []ToolErrorContract    `yaml:"errors,omitempty"`
	Relationships ToolRelationships      `yaml:"relationships,omitempty"`
	Binary        string                 `yaml:"binary,omitempty"`
	Args          []string               `yaml:"args,omitempty"`
	Init          string                 `yaml:"init,omitempty"`
	Config        map[string]interface{} `yaml:"config,omitempty"`
	Emits         []string               `yaml:"emits,omitempty"`
	Parameters    map[string]interface{} `yaml:"parameters,omitempty"`
	Dir           string                 `yaml:"dir,omitempty"`
	Precondition  string                 `yaml:"precondition,omitempty"`
	Visibility    string                 `yaml:"visibility,omitempty"`
	Phases        []string               `yaml:"phases,omitempty"`
	OutputCap     int                    `yaml:"output_cap,omitempty"`
	phaseScoped   bool
}

// ToolRequirements groups observable behaviors a tool must satisfy.
type ToolRequirements struct {
	Input       []string `yaml:"input,omitempty"`
	Output      []string `yaml:"output,omitempty"`
	SideEffects []string `yaml:"side_effects,omitempty"`
	Undo        []string `yaml:"undo,omitempty"`
	Errors      []string `yaml:"errors,omitempty"`
}

// ToolOutputContract describes the structured output shape produced by a tool.
type ToolOutputContract struct {
	Description string                 `yaml:"description,omitempty"`
	Schema      map[string]interface{} `yaml:"schema,omitempty"`
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
	Undo                 string `yaml:"undo,omitempty"`
	RequiresConfirmation bool   `yaml:"requires_confirmation,omitempty"`
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

// ParamMapping holds the extracted CLI mapping for one parameter.
type ParamMapping struct {
	Name       string
	Flag       string
	Positional bool
	BoolFlag   bool
	Default    string
	Required   bool
	Position   int
}

// ToolDefsFile is the top-level YAML structure for declaration files.
type ToolDefsFile struct {
	Includes []string  `yaml:"includes,omitempty"`
	Tools    []ToolDef `yaml:"tools"`
}

// ToolSelectionFile is the YAML structure for a tool selection file.
type ToolSelectionFile struct {
	Tools []string `yaml:"tools"`
}

// ToToolSpec converts a ToolDef to a core.ToolSpec.
func (td *ToolDef) ToToolSpec() core.ToolSpec {
	vis := core.External
	if td.Visibility == "internal" {
		vis = core.Internal
	}
	desc := td.Description
	if td.SideEffects.LegacyText != "" {
		desc += " Side effects: " + td.SideEffects.LegacyText
	}
	return core.ToolSpec{
		Name:        td.Name,
		Description: desc,
		InputSchema: td.buildInputSchema(),
		Visibility:  vis,
		Phases:      toolSpecPhases(td.Phases),
		PhaseScoped: td.phaseScoped || len(td.Phases) > 0,
	}
}

func toolSpecPhases(phases []string) []core.State {
	if len(phases) == 0 {
		return nil
	}
	out := make([]core.State, 0, len(phases))
	for _, phase := range phases {
		out = append(out, core.State(phase))
	}
	return out
}

func (td *ToolDef) buildInputSchema() json.RawMessage {
	if len(td.Parameters) == 0 {
		return json.RawMessage(`{"type":"object","properties":{}}`)
	}
	data, _ := json.Marshal(stripCLIExtensions(td.Parameters))
	return data
}

func stripCLIExtensions(schema map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{}, len(schema))
	for k, v := range schema {
		if cliExtensionKeys[k] {
			continue
		}
		if k == "properties" {
			if props, ok := v.(map[string]interface{}); ok {
				cleaned := make(map[string]interface{}, len(props))
				for pName, pVal := range props {
					if pMap, ok := pVal.(map[string]interface{}); ok {
						cleaned[pName] = stripCLIExtensions(pMap)
					} else {
						cleaned[pName] = pVal
					}
				}
				result[k] = cleaned
				continue
			}
		}
		result[k] = v
	}
	return result
}

// ExtractParamMappings extracts CLI mapping information from parameters.
func (td *ToolDef) ExtractParamMappings() []ParamMapping {
	if len(td.Parameters) == 0 {
		return nil
	}
	props, _ := td.Parameters["properties"].(map[string]interface{})
	if props == nil {
		return nil
	}
	requiredSet := make(map[string]bool)
	if reqList, ok := td.Parameters["required"].([]interface{}); ok {
		for _, r := range reqList {
			if s, ok := r.(string); ok {
				requiredSet[s] = true
			}
		}
	}
	var mappings []ParamMapping
	for name, val := range props {
		pm := ParamMapping{Name: name, Required: requiredSet[name]}
		pMap, ok := val.(map[string]interface{})
		if !ok {
			mappings = append(mappings, pm)
			continue
		}
		if f, ok := pMap["flag"].(string); ok {
			pm.Flag = f
		}
		if b, ok := pMap["positional"].(bool); ok {
			pm.Positional = b
		}
		if b, ok := pMap["bool_flag"].(bool); ok {
			pm.BoolFlag = b
		}
		if d, ok := pMap["default"].(string); ok {
			pm.Default = d
		}
		if position, ok := pMap["position"].(int); ok {
			pm.Position = position
		}
		mappings = append(mappings, pm)
	}
	sort.Slice(mappings, func(i, j int) bool {
		left, right := mappings[i], mappings[j]
		switch {
		case left.Position > 0 && right.Position > 0 && left.Position != right.Position:
			return left.Position < right.Position
		case left.Position > 0 && right.Position == 0:
			return true
		case left.Position == 0 && right.Position > 0:
			return false
		default:
			return left.Name < right.Name
		}
	})
	return mappings
}

// MergeToolDefs combines slices, with later entries overriding earlier ones.
func MergeToolDefs(slices ...[]ToolDef) []ToolDef {
	seen := make(map[string]int)
	var result []ToolDef
	for _, slice := range slices {
		for _, td := range slice {
			if idx, ok := seen[td.Name]; ok {
				result[idx] = td
			} else {
				seen[td.Name] = len(result)
				result = append(result, td)
			}
		}
	}
	return result
}
