// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package catalog

import (
	"encoding/json"
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/yamlstrict"
)

var cliExtensionKeys = map[string]bool{
	"flag":       true,
	"positional": true,
	"bool_flag":  true,
	"default":    true,
	"position":   true,
	"source":     true,
}

// ToolDef is a declarative, YAML-driven tool definition.
type ToolDef struct {
	Name           string                 `yaml:"name"`
	Type           string                 `yaml:"type,omitempty"`
	Category       string                 `yaml:"category,omitempty"`
	Contract       string                 `yaml:"contract,omitempty"`
	Description    string                 `yaml:"description"`
	Problem        string                 `yaml:"problem,omitempty"`
	Goals          []string               `yaml:"goals,omitempty"`
	Requirements   ToolRequirements       `yaml:"requirements,omitempty"`
	NonGoals       []string               `yaml:"non_goals,omitempty"`
	Output         ToolOutputContract     `yaml:"output,omitempty"`
	Metrics        core.MetricConfig      `yaml:"metrics,omitempty"`
	SideEffects    ToolSideEffects        `yaml:"side_effects,omitempty"`
	Reversibility  ToolReversibility      `yaml:"reversibility,omitempty"`
	Undo           ToolUndoContract       `yaml:"undo,omitempty"`
	Errors         []ToolErrorContract    `yaml:"errors,omitempty"`
	Relationships  ToolRelationships      `yaml:"relationships,omitempty"`
	Binary         string                 `yaml:"binary,omitempty"`
	Args           []string               `yaml:"args,omitempty"`
	Init           string                 `yaml:"init,omitempty"`
	Config         map[string]interface{} `yaml:"config,omitempty"`
	Emits          []string               `yaml:"emits,omitempty"`
	Signature      *ToolSignature         `yaml:"signature,omitempty"`
	Parameters     map[string]interface{} `yaml:"parameters,omitempty"`
	Dir            string                 `yaml:"dir,omitempty"`
	Precondition   string                 `yaml:"precondition,omitempty"`
	Visibility     string                 `yaml:"visibility,omitempty"`
	Phases         []string               `yaml:"phases,omitempty"`
	OutputCap      int                    `yaml:"output_cap,omitempty"`
	StdinSource    string                 `yaml:"stdin_source,omitempty"`
	StdinMaxBytes  int                    `yaml:"stdin_max_bytes,omitempty"`
	Env            []string               `yaml:"env,omitempty"`
	Override       bool                   `yaml:"override,omitempty"`
	stdinSourceSet bool
	stdinLimitSet  bool
	envSet         bool
	phaseScoped    bool
	sourceUnit     string
	sourcePath     string
	overrideTarget ToolSource
	fragment       string
	fragmentAs     string
	fragmentArgs   map[string]string
}

// UnmarshalYAML validates command-state selectors while declarations are loaded,
// before an exec tool can be registered or dispatched (srd023 R4.5).
func (td *ToolDef) UnmarshalYAML(value *yaml.Node) error {
	type rawToolDef ToolDef
	if err := yamlstrict.CheckFields(value, yamlstrict.TagsOf(rawToolDef{})...); err != nil {
		return fmt.Errorf("tool definition: %w", err)
	}
	if err := yamlstrict.CheckKnownFields(value, rawToolDef{}); err != nil {
		return fmt.Errorf("tool definition: %w", err)
	}
	var decoded rawToolDef
	if err := value.Decode(&decoded); err != nil {
		return err
	}
	*td = ToolDef(decoded)
	td.stdinSourceSet = yamlstrict.FieldPresent(value, "stdin_source")
	td.stdinLimitSet = yamlstrict.FieldPresent(value, "stdin_max_bytes")
	td.envSet = yamlstrict.FieldPresent(value, "env")
	if err := td.validateParamSources(); err != nil {
		return err
	}
	if err := td.validateParameterExtensions(); err != nil {
		return err
	}
	return td.validateExecIO()
}

func (td ToolDef) validateParameterExtensions() error {
	props, _ := td.Parameters["properties"].(map[string]interface{})
	for name, value := range props {
		property, _ := value.(map[string]interface{})
		if raw, ok := property["default"]; ok {
			if _, stringValue := raw.(string); !stringValue {
				return fmt.Errorf("tool %q parameter %q default must be a string", td.Name, name)
			}
		}
		if raw, ok := property["position"]; ok {
			if _, integer := raw.(int); !integer {
				return fmt.Errorf("tool %q parameter %q position must be an integer", td.Name, name)
			}
		}
		if td.Type == "builtin" {
			for _, field := range []string{"source", "flag", "positional", "position"} {
				if _, present := property[field]; present {
					return fmt.Errorf(
						"builtin tool %q parameter %q cannot declare exec field %q",
						td.Name, name, field,
					)
				}
			}
		}
	}
	return nil
}

func (td ToolDef) validateParamSources() error {
	if td.Type == "builtin" {
		return nil
	}
	props, _ := td.Parameters["properties"].(map[string]interface{})
	for name, value := range props {
		property, _ := value.(map[string]interface{})
		raw, exists := property["source"]
		if !exists {
			continue
		}
		source, ok := raw.(string)
		if !ok {
			return fmt.Errorf("tool %q parameter %q source must be a $from(label).path selector", td.Name, name)
		}
		if _, _, ok := core.ParseFromSelector(source); !ok {
			return fmt.Errorf("tool %q parameter %q source %q must be a $from(label).path selector", td.Name, name, source)
		}
	}
	return nil
}

func (td ToolDef) validateExecIO() error {
	hasExecIO := td.stdinSourceSet || td.stdinLimitSet || td.Output.Mode != ""
	hasEnv := td.envSet || len(td.Env) > 0
	if td.Type == "builtin" {
		if hasExecIO {
			return fmt.Errorf("builtin tool %q cannot declare exec input or output mode", td.Name)
		}
		if hasEnv {
			return fmt.Errorf("builtin tool %q cannot declare env", td.Name)
		}
		return nil
	}
	if hasExecIO && td.Binary == "" {
		return fmt.Errorf("tool %q exec input or output mode requires binary", td.Name)
	}
	if hasEnv && td.Binary == "" {
		return fmt.Errorf("tool %q env requires binary", td.Name)
	}
	if td.stdinSourceSet && td.StdinSource == "" {
		return fmt.Errorf("tool %q stdin_source must not be empty", td.Name)
	}
	if td.StdinSource != "" {
		if _, _, ok := core.ParseFromSelector(td.StdinSource); !ok {
			return fmt.Errorf("tool %q stdin_source %q must be a $from(label).path selector", td.Name, td.StdinSource)
		}
	}
	if td.stdinLimitSet && (td.StdinMaxBytes <= 0 || td.StdinSource == "") {
		return fmt.Errorf("tool %q stdin_max_bytes requires stdin_source and must be positive", td.Name)
	}
	if td.Output.Mode != "" && td.Output.Mode != "raw" && td.Output.Mode != "structured" {
		return fmt.Errorf("tool %q output mode %q must be raw or structured", td.Name, td.Output.Mode)
	}
	return td.validateEnv()
}

// ParamMapping holds the extracted CLI mapping for one parameter.
type ParamMapping struct {
	Name       string
	Flag       string
	Positional bool
	BoolFlag   bool
	// List is a parameter of type array: each element maps to its own
	// flag occurrence or positional argument (srd023 R2.12).
	List     bool
	Default  string
	Source   string
	Required bool
	Position int
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
		Rollback: core.RollbackPolicy{
			Classification: td.Reversibility.Classification,
			Strategy:       td.Undo.Strategy,
			Description:    td.Undo.Description,
			Requires:       append([]string(nil), td.Undo.Requires...),
		},
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
		if t, ok := pMap["type"].(string); ok && t == "array" {
			pm.List = true
		}
		if d, ok := pMap["default"].(string); ok {
			pm.Default = d
		}
		if source, ok := pMap["source"].(string); ok {
			pm.Source = source
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
