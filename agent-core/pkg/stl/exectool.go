// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
)

// CLI mapping extension keys embedded in JSON Schema properties.
// These are stripped before sending the schema to the LLM.
var cliExtensionKeys = map[string]bool{
	"flag":       true,
	"positional": true,
	"bool_flag":  true,
	"default":    true,
}

// ToolDef is a declarative, YAML-driven tool definition. It supports two types:
//
//   - exec (default): wraps a CLI binary. Binary and Args are required.
//   - builtin: delegates to a Go factory function. Init names the factory
//     registered in a BuiltinRegistry. Config passes tool-specific settings.
//
// The parameters field uses JSON Schema format (same as the LLM tool calling
// spec) with CLI mapping extensions on each property.
type ToolDef struct {
	Name         string                 `yaml:"name"`
	Type         string                 `yaml:"type,omitempty"`
	Description  string                 `yaml:"description"`
	Binary       string                 `yaml:"binary,omitempty"`
	Args         []string               `yaml:"args,omitempty"`
	Init         string                 `yaml:"init,omitempty"`
	Config       map[string]interface{} `yaml:"config,omitempty"`
	Parameters   map[string]interface{} `yaml:"parameters,omitempty"`
	Dir          string                 `yaml:"dir,omitempty"`
	Precondition string                 `yaml:"precondition,omitempty"`
	Visibility   string                 `yaml:"visibility,omitempty"`
	OutputCap    int                    `yaml:"output_cap,omitempty"`
	SideEffects  string                 `yaml:"side_effects,omitempty"`
}

// ParamMapping holds the extracted CLI mapping for one parameter.
type ParamMapping struct {
	Name       string
	Flag       string
	Positional bool
	BoolFlag   bool
	Default    string
	Required   bool
}

// ToolDefsFile is the top-level YAML structure.
type ToolDefsFile struct {
	Tools []ToolDef `yaml:"tools"`
}

// LoadToolDefs reads a YAML file and returns the tool definitions.
func LoadToolDefs(path string) ([]ToolDef, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load tool defs %s: %w", path, err)
	}
	return ParseToolDefs(data)
}

// ParseToolDefs parses YAML bytes into tool definitions.
func ParseToolDefs(data []byte) ([]ToolDef, error) {
	var file ToolDefsFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse tool defs: %w", err)
	}
	for i, td := range file.Tools {
		if td.Name == "" {
			return nil, fmt.Errorf("tool at index %d has no name", i)
		}
		switch td.Type {
		case "builtin":
			if td.Init == "" {
				return nil, fmt.Errorf("builtin tool %q has no init field", td.Name)
			}
		case "exec", "":
			if td.Binary == "" {
				return nil, fmt.Errorf("tool %q has no binary", td.Name)
			}
		default:
			return nil, fmt.Errorf("tool %q: unknown type %q", td.Name, td.Type)
		}
	}
	return file.Tools, nil
}

// RegisterToolDefs registers all tool definitions with the given registry.
// root is the working directory for all tools (overridden by ToolDef.Dir).
func RegisterToolDefs(reg *core.Registry, root string, defs []ToolDef) {
	for _, td := range defs {
		spec := td.ToToolSpec()
		builder := &ExecBuilder{Def: td, Root: root}
		reg.Register(spec, builder)
	}
}

// ToToolSpec converts a ToolDef to a core.ToolSpec.
func (td *ToolDef) ToToolSpec() core.ToolSpec {
	vis := core.External
	if td.Visibility == "internal" {
		vis = core.Internal
	}

	desc := td.Description
	if td.SideEffects != "" {
		desc += " Side effects: " + td.SideEffects
	}

	return core.ToolSpec{
		Name:        td.Name,
		Description: desc,
		InputSchema: td.buildInputSchema(),
		Visibility:  vis,
	}
}

// buildInputSchema produces the LLM-facing JSON Schema by stripping
// CLI mapping extensions from the parameters block.
func (td *ToolDef) buildInputSchema() json.RawMessage {
	if len(td.Parameters) == 0 {
		return json.RawMessage(`{"type":"object","properties":{}}`)
	}

	cleaned := stripCLIExtensions(td.Parameters)
	data, _ := json.Marshal(cleaned)
	return data
}

// stripCLIExtensions recursively removes CLI mapping keys from a
// JSON Schema map, returning a clean copy for the LLM.
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

// ExtractParamMappings extracts CLI mapping information from the
// parameters JSON Schema.
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
		pm := ParamMapping{Name: name}
		pm.Required = requiredSet[name]

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

		mappings = append(mappings, pm)
	}

	return mappings
}

// ExecBuilder is the generic Builder for YAML-defined tools.
type ExecBuilder struct {
	Def  ToolDef
	Root string
}

// Build extracts parameters from the previous result and creates an ExecCmd.
func (b *ExecBuilder) Build(res core.Result) core.Command {
	mappings := b.Def.ExtractParamMappings()
	params := make(map[string]string)

	for _, pm := range mappings {
		val := ExtractStringParam(res.Output, pm.Name)
		if val == "" && pm.Default != "" {
			val = pm.Default
		}
		if val == "" && pm.Required {
			return &FailedParamCmd{ToolName: b.Def.Name, Missing: pm.Name}
		}
		if val != "" {
			params[pm.Name] = val
		}
	}
	return &ExecCmd{def: b.Def, root: b.Root, params: params}
}

// ExecCmd is the generic Command for YAML-defined tools.
type ExecCmd struct {
	def    ToolDef
	root   string
	params map[string]string
}

func (c *ExecCmd) Name() string { return c.def.Name }

func (c *ExecCmd) Execute() core.Result {
	dir := c.root
	if c.def.Dir != "" {
		if filepath.IsAbs(c.def.Dir) {
			dir = c.def.Dir
		} else {
			dir = filepath.Join(c.root, c.def.Dir)
		}
	}

	if err := c.checkPrecondition(dir); err != nil {
		return core.Result{
			Output:      err.Error(),
			Signal:      core.ToolFailed,
			CommandName: c.def.Name,
		}
	}

	args := c.buildArgs()

	cmd := exec.Command(c.def.Binary, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()

	res := SubprocessResult(c.def.Name, out, err)
	if c.def.OutputCap > 0 {
		res.Output = CapOutput(res.Output, c.def.OutputCap)
	}
	return res
}

func (c *ExecCmd) buildArgs() []string {
	args := make([]string, len(c.def.Args))
	copy(args, c.def.Args)

	mappings := c.def.ExtractParamMappings()
	for _, pm := range mappings {
		val, ok := c.params[pm.Name]
		if !ok {
			continue
		}
		if pm.BoolFlag {
			args = append(args, pm.Flag)
		} else if pm.Positional {
			args = append(args, val)
		} else {
			args = append(args, pm.Flag, val)
		}
	}

	return args
}

func (c *ExecCmd) checkPrecondition(dir string) error {
	switch c.def.Precondition {
	case "git_repo":
		return verifyGitDir(dir)
	case "":
		return nil
	default:
		gitPath := filepath.Join(dir, ".git")
		if _, err := os.Stat(gitPath); err != nil {
			return fmt.Errorf("precondition %q failed: %v", c.def.Precondition, err)
		}
		return nil
	}
}

// MergeToolDefs combines multiple tool definition slices, later entries
// override earlier ones with the same name.
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
