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
	OutputCap     int                    `yaml:"output_cap,omitempty"`
}

// ToolRequirements groups observable behaviors a tool must satisfy. These
// fields describe the tool contract; runtime validation is added separately.
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

// ToolSideEffects accepts both the legacy scalar side_effects string and the
// structured list form. The scalar is kept for migration compatibility.
type ToolSideEffects struct {
	LegacyText string
	Items      []ToolSideEffect
}

// UnmarshalYAML supports legacy:
//
//	side_effects: "creates files"
//
// and structured:
//
//	side_effects:
//	  - kind: filesystem_write
//	    paths: ["out.txt"]
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

// ToolUndoContract describes how to reverse or compensate for tool effects.
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

// ToolRelationships documents common composition neighbors and overlapping
// tools. It guides agents and humans but does not constrain state machines.
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
}

// ToolDefsFile is the top-level YAML structure. The optional Includes
// field lists relative paths to other tool definition files whose tools
// are loaded first; the current file's tools override included ones
// with the same name.
type ToolDefsFile struct {
	Includes []string  `yaml:"includes,omitempty"`
	Tools    []ToolDef `yaml:"tools"`
}

// ToolSelectionFile is the YAML structure for a tool selection file.
// It lists tool names that an agent is allowed to use — a subset of
// the tools loaded via declaration files.
type ToolSelectionFile struct {
	Tools []string `yaml:"tools"`
}

// LoadToolSelection reads a YAML file listing tool names.
func LoadToolSelection(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load tool selection %s: %w", path, err)
	}
	var sel ToolSelectionFile
	if err := yaml.Unmarshal(data, &sel); err != nil {
		return nil, fmt.Errorf("parse tool selection %s: %w", path, err)
	}
	return sel.Tools, nil
}

// LoadToolSelections reads multiple tool selection YAML files and merges
// their tool name lists, deduplicating entries.
func LoadToolSelections(paths []string) ([]string, error) {
	seen := map[string]bool{}
	var merged []string
	for _, p := range paths {
		names, err := LoadToolSelection(p)
		if err != nil {
			return nil, err
		}
		for _, n := range names {
			if !seen[n] {
				seen[n] = true
				merged = append(merged, n)
			}
		}
	}
	return merged, nil
}

// LoadToolDeclarations loads multiple declaration files and merges them.
// Later files override earlier ones with the same tool name.
func LoadToolDeclarations(paths []string) ([]ToolDef, error) {
	var all []ToolDef
	for _, p := range paths {
		defs, err := LoadToolDefs(p)
		if err != nil {
			return nil, err
		}
		all = MergeToolDefs(all, defs)
	}
	return all, nil
}

// LoadToolDeclarationsFromDirs scans directories for *.yaml files and
// loads them as tool declarations. Files are sorted by name within each
// directory. Results are merged with later directories overriding earlier ones.
func LoadToolDeclarationsFromDirs(dirs []string) ([]ToolDef, error) {
	var paths []string
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, fmt.Errorf("scan tool config dir %s: %w", dir, err)
		}
		for _, e := range entries {
			if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
				continue
			}
			paths = append(paths, filepath.Join(dir, e.Name()))
		}
	}
	return LoadToolDeclarations(paths)
}

// SelectTools filters a set of declarations to only those named in the
// selection list. Returns an error if any selected name is not declared.
func SelectTools(declarations []ToolDef, selection []string) ([]ToolDef, error) {
	index := make(map[string]ToolDef, len(declarations))
	for _, d := range declarations {
		index[d.Name] = d
	}
	var result []ToolDef
	for _, name := range selection {
		d, ok := index[name]
		if !ok {
			return nil, fmt.Errorf("tool %q is selected but not declared", name)
		}
		result = append(result, d)
	}
	return result, nil
}

// LoadToolDefs reads a YAML file and returns the tool definitions.
// If the file has an `includes` field, included files are loaded first
// (relative to the directory of the including file) and merged so that
// the current file's definitions take precedence.
func LoadToolDefs(path string) ([]ToolDef, error) {
	return loadToolDefsRecursive(path, nil)
}

func loadToolDefsRecursive(path string, seen map[string]bool) ([]ToolDef, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve path %s: %w", path, err)
	}
	if seen == nil {
		seen = make(map[string]bool)
	}
	if seen[abs] {
		return nil, fmt.Errorf("circular include detected: %s", abs)
	}
	seen[abs] = true

	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, fmt.Errorf("load tool defs %s: %w", abs, err)
	}

	var file ToolDefsFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse tool defs %s: %w", abs, err)
	}

	var base []ToolDef
	dir := filepath.Dir(abs)
	for _, inc := range file.Includes {
		incPath := inc
		if !filepath.IsAbs(incPath) {
			incPath = filepath.Join(dir, incPath)
		}
		incDefs, err := loadToolDefsRecursive(incPath, seen)
		if err != nil {
			return nil, fmt.Errorf("include %s from %s: %w", inc, abs, err)
		}
		base = MergeToolDefs(base, incDefs)
	}

	if err := validateToolDefs(file.Tools); err != nil {
		return nil, err
	}

	return MergeToolDefs(base, file.Tools), nil
}

// ParseToolDefs parses YAML bytes into tool definitions.
// Note: includes are not resolved when parsing from bytes.
func ParseToolDefs(data []byte) ([]ToolDef, error) {
	var file ToolDefsFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse tool defs: %w", err)
	}
	return file.Tools, validateToolDefs(file.Tools)
}

func validateToolDefs(defs []ToolDef) error {
	for i, td := range defs {
		if td.Name == "" {
			return fmt.Errorf("tool at index %d has no name", i)
		}
		switch td.Type {
		case "builtin":
			if td.Init == "" {
				return fmt.Errorf("builtin tool %q has no init field", td.Name)
			}
		case "exec", "":
			if td.Binary == "" {
				return fmt.Errorf("tool %q has no binary", td.Name)
			}
		default:
			return fmt.Errorf("tool %q: unknown type %q", td.Name, td.Type)
		}
	}
	return nil
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
	if td.SideEffects.LegacyText != "" {
		desc += " Side effects: " + td.SideEffects.LegacyText
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
func (c *ExecCmd) Undo() core.Result {
	switch c.def.Undo.Strategy {
	case "", "noop":
		return core.NoopUndo(c.Name())
	case "workspace_restore":
		return core.Result{
			Signal:      core.ToolDone,
			CommandName: c.Name(),
			Output:      "undo: workspace restore is handled by the rollback workspace layer",
		}
	case "compensating_action":
		err := fmt.Errorf("undo %s requires compensating action: %s", c.Name(), c.def.Undo.Description)
		return core.Result{
			Signal:      core.CommandError,
			CommandName: c.Name(),
			Output:      err.Error(),
			Err:         err,
		}
	default:
		err := fmt.Errorf("undo %s: unsupported undo strategy %q", c.Name(), c.def.Undo.Strategy)
		return core.Result{
			Signal:      core.CommandError,
			CommandName: c.Name(),
			Output:      err.Error(),
			Err:         err,
		}
	}
}

func (c *ExecCmd) UndoMemento() (core.UndoMemento, error) {
	switch c.def.Undo.Strategy {
	case "", "noop":
		return core.NoopUndoMemento(c.Name()), nil
	case "workspace_restore":
		return core.NewUndoMemento(c.Name(), core.UndoMementoReversible, workspaceRestorePayload(c.def))
	case "compensating_action":
		payload := any(workspaceRestorePayload(c.def))
		if c.def.Undo.Payload == "boundary_compensation" {
			payload = execBoundaryCompensationPayload(c.def, c.params)
		}
		memento, err := core.NewUndoMemento(c.Name(), core.UndoMementoCompensatable, payload)
		if err != nil {
			return core.UndoMemento{}, err
		}
		memento.Description = c.def.Undo.Description
		return memento, nil
	default:
		return core.UndoMemento{}, fmt.Errorf("%w: unsupported undo strategy %q for %s", core.ErrUndoMementoIncompatible, c.def.Undo.Strategy, c.Name())
	}
}

func execBoundaryCompensationPayload(def ToolDef, params map[string]string) BoundaryCompensationPayload {
	workspacePayload := workspaceRestorePayload(def)
	compensation := BoundaryCompensation{
		Strategy:       def.Undo.Strategy,
		Reason:         def.Undo.Description,
		Requires:       append([]string(nil), def.Undo.Requires...),
		WorkspacePaths: append([]string(nil), workspacePayload.WorkspaceRestore.Paths...),
		IssueID:        params["id"],
	}
	return BoundaryCompensationPayload{BoundaryCompensation: compensation}
}

func workspaceRestorePayload(def ToolDef) workspaceUndoPayload {
	payload := workspaceUndoPayload{}
	for _, effect := range def.SideEffects.Items {
		payload.WorkspaceRestore.Paths = append(payload.WorkspaceRestore.Paths, effect.Paths...)
	}
	if len(payload.WorkspaceRestore.Paths) == 0 {
		payload.WorkspaceRestore.Paths = []string{"."}
	}
	return payload
}

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
		gitPath := filepath.Join(dir, ".git")
		if _, err := os.Stat(gitPath); err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("not a git repository: %s", dir)
			}
			return fmt.Errorf("checking git repo %s: %v", dir, err)
		}
		return nil
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
