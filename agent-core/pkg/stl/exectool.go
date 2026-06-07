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

// ToolDef is a declarative, YAML-driven definition for a CLI-wrapping tool.
// Instead of writing Go structs per tool, define them in YAML and let the
// generic ExecCmd + ExecBuilder handle execution and parameter mapping.
type ToolDef struct {
	Name        string     `yaml:"name"`
	Description string     `yaml:"description"`
	Binary      string     `yaml:"binary"`
	Args        []string   `yaml:"args"`
	Params      []ParamDef `yaml:"params,omitempty"`
	Dir         string     `yaml:"dir,omitempty"`
	Precondition string   `yaml:"precondition,omitempty"`
	Visibility  string     `yaml:"visibility,omitempty"`
	OutputCap   int        `yaml:"output_cap,omitempty"`
	SideEffects string     `yaml:"side_effects,omitempty"`
}

// ParamDef maps a tool input parameter to a CLI flag.
type ParamDef struct {
	Name     string `yaml:"name"`
	Flag     string `yaml:"flag"`
	Required bool   `yaml:"required,omitempty"`
	Default  string `yaml:"default,omitempty"`
	Desc     string `yaml:"description,omitempty"`
	// Positional, when true, appends the value without a flag prefix.
	Positional bool `yaml:"positional,omitempty"`
	// BoolFlag, when true, appends the flag without a value when param is present.
	BoolFlag bool `yaml:"bool_flag,omitempty"`
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
		if td.Binary == "" {
			return nil, fmt.Errorf("tool %q has no binary", td.Name)
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

func (td *ToolDef) buildInputSchema() json.RawMessage {
	if len(td.Params) == 0 {
		return json.RawMessage(`{"type":"object","properties":{}}`)
	}

	props := make(map[string]map[string]string)
	var required []string

	for _, p := range td.Params {
		prop := map[string]string{"type": "string"}
		if p.Desc != "" {
			prop["description"] = p.Desc
		}
		props[p.Name] = prop
		if p.Required {
			required = append(required, p.Name)
		}
	}

	schema := map[string]interface{}{
		"type":       "object",
		"properties": props,
	}
	if len(required) > 0 {
		schema["required"] = required
	}

	data, _ := json.Marshal(schema)
	return data
}

// ExecBuilder is the generic Builder for YAML-defined tools.
type ExecBuilder struct {
	Def  ToolDef
	Root string
}

// Build extracts parameters from the previous result and creates an ExecCmd.
func (b *ExecBuilder) Build(res core.Result) core.Command {
	params := make(map[string]string)
	for _, p := range b.Def.Params {
		val := ExtractStringParam(res.Output, p.Name)
		if val == "" && p.Default != "" {
			val = p.Default
		}
		if val == "" && p.Required {
			return &FailedParamCmd{ToolName: b.Def.Name, Missing: p.Name}
		}
		if val != "" {
			params[p.Name] = val
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

	for _, p := range c.def.Params {
		val, ok := c.params[p.Name]
		if !ok {
			continue
		}
		if p.BoolFlag {
			args = append(args, p.Flag)
		} else if p.Positional {
			args = append(args, val)
		} else {
			args = append(args, p.Flag, val)
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
