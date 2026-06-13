// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
)

const testToolsYAML = `
tools:
  - name: greet
    binary: echo
    args: [hello]
    emits: [ToolDone, ToolFailed]
    description: "Say hello"
    parameters:
      type: object
      properties:
        name:
          type: string
          description: "Name to greet"
          flag: --name
        loud:
          type: boolean
          description: "Shout"
          flag: --loud
          bool_flag: true
      required: [name]
  - name: list_dir
    binary: ls
    args: [-la]
    description: "List directory"
    parameters:
      type: object
      properties:
        path:
          type: string
          description: "Directory to list"
          positional: true
          default: "."
`

func TestParseToolDefs(t *testing.T) {
	defs, err := ParseToolDefs([]byte(testToolsYAML))
	require.NoError(t, err)
	assert.Len(t, defs, 2)

	assert.Equal(t, "greet", defs[0].Name)
	assert.Equal(t, "echo", defs[0].Binary)
	assert.Equal(t, []string{"hello"}, defs[0].Args)
	assert.Equal(t, []string{"ToolDone", "ToolFailed"}, defs[0].Emits)

	mappings := defs[0].ExtractParamMappings()
	assert.Len(t, mappings, 2)

	nameMapping := findMapping(mappings, "name")
	require.NotNil(t, nameMapping)
	assert.Equal(t, "--name", nameMapping.Flag)
	assert.True(t, nameMapping.Required)

	loudMapping := findMapping(mappings, "loud")
	require.NotNil(t, loudMapping)
	assert.True(t, loudMapping.BoolFlag)

	assert.Equal(t, "list_dir", defs[1].Name)
	pathMappings := defs[1].ExtractParamMappings()
	pathMapping := findMapping(pathMappings, "path")
	require.NotNil(t, pathMapping)
	assert.True(t, pathMapping.Positional)
	assert.Equal(t, ".", pathMapping.Default)
}

func findMapping(mappings []ParamMapping, name string) *ParamMapping {
	for i := range mappings {
		if mappings[i].Name == name {
			return &mappings[i]
		}
	}
	return nil
}

func TestParseToolDefs_Errors(t *testing.T) {
	tests := []struct {
		name  string
		yaml  string
		errIs string
	}{
		{
			name:  "missing name",
			yaml:  "tools:\n  - binary: echo",
			errIs: "no name",
		},
		{
			name:  "missing binary",
			yaml:  "tools:\n  - name: foo",
			errIs: "no binary",
		},
		{
			name:  "invalid yaml",
			yaml:  "tools: [[[",
			errIs: "parse tool defs",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseToolDefs([]byte(tt.yaml))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errIs)
		})
	}
}

func TestToolDef_ToToolSpec(t *testing.T) {
	td := ToolDef{
		Name:        "build",
		Description: "Compile stuff.",
		Binary:      "go",
		Args:        []string{"build"},
		SideEffects: ToolSideEffects{
			LegacyText: "produces binary",
		},
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"pkg": map[string]interface{}{
					"type":        "string",
					"description": "Package",
					"flag":        "--pkg",
				},
			},
			"required": []interface{}{"pkg"},
		},
	}

	spec := td.ToToolSpec()
	assert.Equal(t, "build", spec.Name)
	assert.Contains(t, spec.Description, "Compile stuff.")
	assert.Contains(t, spec.Description, "Side effects: produces binary")
	assert.Equal(t, core.External, spec.Visibility)

	var schema map[string]interface{}
	require.NoError(t, json.Unmarshal(spec.InputSchema, &schema))
	props := schema["properties"].(map[string]interface{})
	pkg := props["pkg"].(map[string]interface{})
	assert.Equal(t, "string", pkg["type"])
	assert.Equal(t, "Package", pkg["description"])
	// CLI extensions should be stripped
	assert.NotContains(t, pkg, "flag")
}

func TestParseToolDefs_ContractFields(t *testing.T) {
	yaml := `tools:
  - name: parse_csv
    type: exec
    category: word
    binary: csvtool
    description: "Parse a CSV file into rows."
    problem: "The agent needs a typed way to turn CSV files into row data."
    goals:
      - "Return deterministic row data for a single CSV input."
    requirements:
      input:
        - "must accept a path to one CSV file"
      output:
        - "must return row_count and rows"
      side_effects:
        - "must not mutate the workspace"
      undo:
        - "must be a no-op"
      errors:
        - "must fail when the CSV cannot be parsed"
    non_goals:
      - "Does not transform or clean CSV values."
    output:
      description: "Parsed CSV rows."
      schema:
        type: object
        properties:
          row_count:
            type: integer
          rows:
            type: array
    side_effects:
      - kind: none
        description: "Read-only tool."
    reversibility:
      classification: reversible
      undo: noop
      requires_confirmation: false
    undo:
      strategy: noop
      description: "No state is changed."
    errors:
      - signal: ToolFailed
        condition: "CSV parse error"
        message_shape: "parse csv: <error>"
        state_after_failure: unchanged
    relationships:
      before: [read]
      after: [write]
      overlaps:
        - tool: read
          difference: "read returns raw text; parse_csv returns structured rows"
`
	defs, err := ParseToolDefs([]byte(yaml))
	require.NoError(t, err)
	require.Len(t, defs, 1)

	def := defs[0]
	assert.Equal(t, "word", def.Category)
	assert.Equal(t, "The agent needs a typed way to turn CSV files into row data.", def.Problem)
	assert.Equal(t, []string{"Return deterministic row data for a single CSV input."}, def.Goals)
	assert.Equal(t, []string{"must accept a path to one CSV file"}, def.Requirements.Input)
	assert.Equal(t, []string{"Does not transform or clean CSV values."}, def.NonGoals)
	assert.Equal(t, "Parsed CSV rows.", def.Output.Description)
	assert.Equal(t, "object", def.Output.Schema["type"])
	require.Len(t, def.SideEffects.Items, 1)
	assert.Equal(t, "none", def.SideEffects.Items[0].Kind)
	assert.Equal(t, "reversible", def.Reversibility.Classification)
	assert.Equal(t, "noop", def.Undo.Strategy)
	require.Len(t, def.Errors, 1)
	assert.Equal(t, "ToolFailed", def.Errors[0].Signal)
	assert.Equal(t, []string{"read"}, def.Relationships.Before)
	require.Len(t, def.Relationships.Overlaps, 1)
	assert.Equal(t, "read", def.Relationships.Overlaps[0].Tool)
}

func TestParseToolDefs_LegacySideEffectsString(t *testing.T) {
	defs, err := ParseToolDefs([]byte(`tools:
  - name: copy_dir
    type: exec
    binary: cp
    description: "Copy directory"
    side_effects: "creates files in the destination directory"
`))
	require.NoError(t, err)
	require.Len(t, defs, 1)
	assert.Equal(t, "creates files in the destination directory", defs[0].SideEffects.LegacyText)
	assert.Empty(t, defs[0].SideEffects.Items)
}

func TestToolDef_ToToolSpec_IgnoresStructuredContractFields(t *testing.T) {
	td := ToolDef{
		Name:        "parse_csv",
		Description: "Parse CSV.",
		Binary:      "csvtool",
		Problem:     "Need structured CSV rows.",
		Goals:       []string{"Return rows."},
		SideEffects: ToolSideEffects{
			Items: []ToolSideEffect{{Kind: "none", Description: "Read-only."}},
		},
		Reversibility: ToolReversibility{Classification: "reversible", Undo: "noop"},
	}

	spec := td.ToToolSpec()
	assert.Equal(t, "Parse CSV.", spec.Description)
	assert.NotContains(t, spec.Description, "Need structured CSV rows")
	assert.NotContains(t, spec.Description, "Read-only")
}

func TestToolDef_ToToolSpec_Internal(t *testing.T) {
	td := ToolDef{
		Name:       "internal_thing",
		Binary:     "true",
		Visibility: "internal",
	}
	spec := td.ToToolSpec()
	assert.Equal(t, core.Internal, spec.Visibility)
}

func TestStripCLIExtensions(t *testing.T) {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"msg": map[string]interface{}{
				"type":        "string",
				"description": "Message",
				"flag":        "-m",
				"positional":  false,
				"bool_flag":   false,
				"default":     "hello",
			},
		},
		"required": []interface{}{"msg"},
	}

	cleaned := stripCLIExtensions(schema)

	props := cleaned["properties"].(map[string]interface{})
	msg := props["msg"].(map[string]interface{})

	assert.Equal(t, "string", msg["type"])
	assert.Equal(t, "Message", msg["description"])
	assert.NotContains(t, msg, "flag")
	assert.NotContains(t, msg, "positional")
	assert.NotContains(t, msg, "bool_flag")
	assert.NotContains(t, msg, "default")

	assert.Contains(t, cleaned, "required")
	assert.Equal(t, "object", cleaned["type"])
}

func TestExecBuilder_MissingRequired(t *testing.T) {
	td := ToolDef{
		Name:   "greet",
		Binary: "echo",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name": map[string]interface{}{
					"type": "string",
					"flag": "--name",
				},
			},
			"required": []interface{}{"name"},
		},
	}
	builder := &ExecBuilder{Def: td, Root: "/tmp"}
	cmd := builder.Build(core.Result{Output: `{"parameters":{}}`})
	res := cmd.Execute()
	assert.Equal(t, core.ToolFailed, res.Signal)
	assert.Contains(t, res.Output, "name")
}

func TestExecBuilder_WithDefault(t *testing.T) {
	td := ToolDef{
		Name:   "list",
		Binary: "echo",
		Args:   []string{"listing"},
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":       "string",
					"positional": true,
					"default":    ".",
				},
			},
		},
	}
	builder := &ExecBuilder{Def: td, Root: "/tmp"}
	cmd := builder.Build(core.Result{Output: `{"parameters":{}}`})

	ec := cmd.(*ExecCmd)
	assert.Equal(t, ".", ec.params["path"])
}

func TestExecCmd_BuildArgs(t *testing.T) {
	def := ToolDef{
		Name:   "test",
		Binary: "go",
		Args:   []string{"test", "-count=1"},
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"package": map[string]interface{}{
					"type":       "string",
					"positional": true,
				},
				"verbose": map[string]interface{}{
					"type":      "boolean",
					"flag":      "-v",
					"bool_flag": true,
				},
			},
		},
	}

	cmd := &ExecCmd{
		def:    def,
		root:   "/tmp",
		params: map[string]string{"package": "./pkg/...", "verbose": "true"},
	}

	args := cmd.buildArgs()
	// Order depends on map iteration, so check contents
	assert.Contains(t, args, "test")
	assert.Contains(t, args, "-count=1")
	assert.Contains(t, args, "./pkg/...")
	assert.Contains(t, args, "-v")
}

func TestExecCmd_BuildArgs_FlagParams(t *testing.T) {
	def := ToolDef{
		Name:   "create",
		Binary: "bd",
		Args:   []string{"create", "--json"},
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"title": map[string]interface{}{
					"type": "string",
					"flag": "--title",
				},
				"body": map[string]interface{}{
					"type": "string",
					"flag": "--body",
				},
			},
		},
	}

	cmd := &ExecCmd{
		def:    def,
		root:   "/tmp",
		params: map[string]string{"title": "fix bug"},
	}

	args := cmd.buildArgs()
	assert.Contains(t, args, "--title")
	assert.Contains(t, args, "fix bug")
	assert.NotContains(t, args, "--body")
}

func TestExecCmdUndoWorkspaceRestoreIsHandledByWorkspaceLayer(t *testing.T) {
	cmd := &ExecCmd{def: ToolDef{
		Name: "copy_dir",
		Undo: ToolUndoContract{Strategy: "workspace_restore"},
	}}

	res := cmd.Undo()

	require.Equal(t, core.ToolDone, res.Signal)
	assert.Contains(t, res.Output, "workspace restore")
}

func TestExecCmdUndoCompensatingActionReportsGap(t *testing.T) {
	cmd := &ExecCmd{def: ToolDef{
		Name: "issue_create",
		Undo: ToolUndoContract{
			Strategy:    "compensating_action",
			Description: "close created issue",
		},
	}}

	res := cmd.Undo()

	require.Equal(t, core.CommandError, res.Signal)
	require.Error(t, res.Err)
	assert.Contains(t, res.Output, "requires compensating action")
}

func TestExecCmdUndoMementoUsesDeclaredStrategy(t *testing.T) {
	cmd := &ExecCmd{def: ToolDef{
		Name: "copy_dir",
		SideEffects: ToolSideEffects{Items: []ToolSideEffect{{
			Kind:  "filesystem_write",
			Paths: []string{"out"},
		}}},
		Undo: ToolUndoContract{Strategy: "workspace_restore"},
	}}

	memento, err := cmd.UndoMemento()

	require.NoError(t, err)
	require.Equal(t, core.UndoMementoReversible, memento.Kind)
	require.NoError(t, core.ValidateUndoMemento(memento))
	assert.Contains(t, string(memento.Payload), `"out"`)
}

func TestExecCmdUndoMementoUsesBoundaryCompensationPayload(t *testing.T) {
	cmd := &ExecCmd{
		def: ToolDef{
			Name: "issue_close",
			SideEffects: ToolSideEffects{Items: []ToolSideEffect{{
				Kind:  "filesystem_write",
				Paths: []string{".beads"},
			}}},
			Undo: ToolUndoContract{
				Strategy:    "compensating_action",
				Description: "reopen closed issue",
				Payload:     "boundary_compensation",
				Requires:    []string{"issue_id", "previous_issue_status"},
			},
		},
		params: map[string]string{"id": "agent-core-123"},
	}

	memento, err := cmd.UndoMemento()

	require.NoError(t, err)
	require.Equal(t, core.UndoMementoCompensatable, memento.Kind)
	require.NoError(t, core.ValidateUndoMemento(memento))
	assert.Contains(t, string(memento.Payload), `"boundary_compensation"`)
	assert.Contains(t, string(memento.Payload), `"issue_id":"agent-core-123"`)
	assert.Contains(t, string(memento.Payload), `".beads"`)
}

func TestExecCmd_Execute_Success(t *testing.T) {
	def := ToolDef{
		Name:   "greet",
		Binary: "echo",
		Args:   []string{"hello"},
	}
	cmd := &ExecCmd{def: def, root: "/tmp", params: map[string]string{}}
	res := cmd.Execute()

	assert.Equal(t, core.ToolDone, res.Signal)
	assert.Equal(t, "hello", res.Output)
	assert.Equal(t, "greet", res.CommandName)
}

func TestExecCmd_Execute_Failure(t *testing.T) {
	def := ToolDef{
		Name:   "fail",
		Binary: "false",
	}
	cmd := &ExecCmd{def: def, root: "/tmp", params: map[string]string{}}
	res := cmd.Execute()

	assert.Equal(t, core.ToolFailed, res.Signal)
}

func TestExecCmd_Execute_WithOutputCap(t *testing.T) {
	def := ToolDef{
		Name:      "seq",
		Binary:    "seq",
		Args:      []string{"100"},
		OutputCap: 5,
	}
	cmd := &ExecCmd{def: def, root: "/tmp", params: map[string]string{}}
	res := cmd.Execute()

	assert.Equal(t, core.ToolDone, res.Signal)
	assert.Contains(t, res.Output, "omitted")
}

func TestExecCmd_Precondition_GitRepo(t *testing.T) {
	def := ToolDef{
		Name:         "status",
		Binary:       "git",
		Args:         []string{"status"},
		Precondition: "git_repo",
	}
	cmd := &ExecCmd{def: def, root: t.TempDir(), params: map[string]string{}}
	res := cmd.Execute()

	assert.Equal(t, core.ToolFailed, res.Signal)
	assert.Contains(t, res.Output, "not a git repository")
}

func TestRegisterToolDefs(t *testing.T) {
	defs, err := ParseToolDefs([]byte(testToolsYAML))
	require.NoError(t, err)

	reg := core.NewRegistry()
	RegisterToolDefs(reg, "/tmp", defs)

	names := reg.ExternalToolNames()
	assert.Contains(t, names, "greet")
	assert.Contains(t, names, "list_dir")

	_, ok := reg.Resolve("greet")
	assert.True(t, ok)
}

func TestMergeToolDefs(t *testing.T) {
	base := []ToolDef{
		{
			Name:     "build",
			Binary:   "go",
			Args:     []string{"build"},
			Category: "boundary",
			Problem:  "Compile the workspace.",
			Reversibility: ToolReversibility{
				Classification: "reversible",
				Undo:           "noop",
			},
		},
		{Name: "test", Binary: "go", Args: []string{"test"}},
	}
	override := []ToolDef{
		{
			Name:     "build",
			Binary:   "go",
			Args:     []string{"build", "-race"},
			Category: "boundary",
			Problem:  "Compile the workspace with race detector.",
			Reversibility: ToolReversibility{
				Classification: "reversible",
				Undo:           "noop",
			},
		},
		{Name: "lint", Binary: "golangci-lint"},
	}

	merged := MergeToolDefs(base, override)
	assert.Len(t, merged, 3)

	assert.Equal(t, "build", merged[0].Name)
	assert.Equal(t, []string{"build", "-race"}, merged[0].Args)
	assert.Equal(t, "boundary", merged[0].Category)
	assert.Equal(t, "Compile the workspace with race detector.", merged[0].Problem)
	assert.Equal(t, "reversible", merged[0].Reversibility.Classification)
	assert.Equal(t, "test", merged[1].Name)
	assert.Equal(t, "lint", merged[2].Name)
}

func TestLoadDefaultToolDefs(t *testing.T) {
	defs, err := LoadToolDefs("tools.yaml")
	require.NoError(t, err)
	assert.True(t, len(defs) >= 21, "expected at least 21 default tool defs, got %d", len(defs))

	names := make(map[string]bool)
	for _, d := range defs {
		names[d.Name] = true
	}
	for _, expected := range []string{
		"build", "vet", "lint", "test",
		"stage_all", "workspace_status", "commit", "rev_parse",
		"branch_create", "branch_delete", "worktree_add", "worktree_remove",
		"issue_create", "issue_close", "issue_list", "issue_claim",
	} {
		assert.True(t, names[expected], "missing tool %q", expected)
	}
}

func TestDefaultToolDefs_CLIExtensionsStripped(t *testing.T) {
	defs, err := LoadToolDefs("tools.yaml")
	require.NoError(t, err)

	for _, d := range defs {
		spec := d.ToToolSpec()
		if len(spec.InputSchema) == 0 {
			continue
		}

		var schema map[string]interface{}
		require.NoError(t, json.Unmarshal(spec.InputSchema, &schema), "tool %s", d.Name)

		props, ok := schema["properties"].(map[string]interface{})
		if !ok {
			continue
		}
		for pName, pVal := range props {
			pMap, ok := pVal.(map[string]interface{})
			if !ok {
				continue
			}
			for ext := range cliExtensionKeys {
				assert.NotContains(t, pMap, ext,
					"tool %s property %s should not have CLI extension %q in LLM schema",
					d.Name, pName, ext)
			}
		}
	}
}

func TestExtractParamMappings_Empty(t *testing.T) {
	td := ToolDef{Name: "noop", Binary: "true"}
	assert.Nil(t, td.ExtractParamMappings())
}

func TestExtractParamMappings_Full(t *testing.T) {
	td := ToolDef{
		Name:   "test",
		Binary: "go",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"pkg": map[string]interface{}{
					"type":       "string",
					"positional": true,
					"default":    "./...",
				},
				"verbose": map[string]interface{}{
					"type":      "boolean",
					"flag":      "-v",
					"bool_flag": true,
				},
			},
			"required": []interface{}{"pkg"},
		},
	}

	mappings := td.ExtractParamMappings()
	assert.Len(t, mappings, 2)

	pkg := findMapping(mappings, "pkg")
	require.NotNil(t, pkg)
	assert.True(t, pkg.Positional)
	assert.True(t, pkg.Required)
	assert.Equal(t, "./...", pkg.Default)

	verbose := findMapping(mappings, "verbose")
	require.NotNil(t, verbose)
	assert.True(t, verbose.BoolFlag)
	assert.Equal(t, "-v", verbose.Flag)
	assert.False(t, verbose.Required)
}

func TestLoadToolSelection(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/tools.yaml"
	writeFile(t, path, `tools:
  - read
  - write
  - build
`)
	names, err := LoadToolSelection(path)
	require.NoError(t, err)
	assert.Equal(t, []string{"read", "write", "build"}, names)
}

func TestLoadToolSelections(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, dir+"/a.yaml", "tools:\n  - read\n  - write\n")
	writeFile(t, dir+"/b.yaml", "tools:\n  - build\n  - write\n")

	names, err := LoadToolSelections([]string{dir + "/a.yaml", dir + "/b.yaml"})
	require.NoError(t, err)
	assert.Equal(t, []string{"read", "write", "build"}, names)
}

func TestLoadToolSelections_Single(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir+"/tools.yaml", "tools:\n  - read\n  - write\n")

	names, err := LoadToolSelections([]string{dir + "/tools.yaml"})
	require.NoError(t, err)
	assert.Equal(t, []string{"read", "write"}, names)
}

func TestLoadToolDeclarationsFromDirs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "read.yaml"), `tools:
  - name: read
    type: builtin
    init: file_read
    description: Read a file
`)
	writeFile(t, filepath.Join(dir, "write.yaml"), `tools:
  - name: write
    type: builtin
    init: file_write
    description: Write a file
`)
	writeFile(t, filepath.Join(dir, "not-yaml.txt"), "ignored")
	require.NoError(t, os.Mkdir(filepath.Join(dir, "subdir"), 0o755))

	defs, err := LoadToolDeclarationsFromDirs([]string{dir})
	require.NoError(t, err)
	require.Len(t, defs, 2)
	assert.Equal(t, "read", defs[0].Name)
	assert.Equal(t, "write", defs[1].Name)
}

func TestLoadToolDeclarationsFromDirs_MissingDir(t *testing.T) {
	_, err := LoadToolDeclarationsFromDirs([]string{"/nonexistent/dir"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "scan tool config dir")
}

func TestSelectTools(t *testing.T) {
	decls := []ToolDef{
		{Name: "read", Type: "builtin", Init: "file_read", Description: "Read files"},
		{Name: "write", Type: "builtin", Init: "file_write", Description: "Write files"},
		{Name: "build", Type: "exec", Binary: "go", Args: []string{"build"}, Description: "Go build"},
	}

	t.Run("valid selection", func(t *testing.T) {
		result, err := SelectTools(decls, []string{"read", "build"})
		require.NoError(t, err)
		require.Len(t, result, 2)
		assert.Equal(t, "read", result[0].Name)
		assert.Equal(t, "build", result[1].Name)
		assert.Equal(t, "builtin", result[0].Type)
		assert.Equal(t, "exec", result[1].Type)
	})

	t.Run("undeclared tool", func(t *testing.T) {
		_, err := SelectTools(decls, []string{"read", "missing"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing")
		assert.Contains(t, err.Error(), "not declared")
	})

	t.Run("empty selection", func(t *testing.T) {
		result, err := SelectTools(decls, []string{})
		require.NoError(t, err)
		assert.Empty(t, result)
	})
}

func TestLoadToolDeclarations_Merge(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir+"/a.yaml", `tools:
  - name: foo
    type: exec
    binary: echo
    args: [a]
    description: "Foo A"
`)
	writeFile(t, dir+"/b.yaml", `tools:
  - name: foo
    type: exec
    binary: echo
    args: [b]
    description: "Foo B"
  - name: bar
    type: exec
    binary: echo
    args: [bar]
    description: "Bar"
`)
	defs, err := LoadToolDeclarations([]string{dir + "/a.yaml", dir + "/b.yaml"})
	require.NoError(t, err)
	require.Len(t, defs, 2)
	assert.Equal(t, "foo", defs[0].Name)
	assert.Equal(t, "Foo B", defs[0].Description)
	assert.Equal(t, "bar", defs[1].Name)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
}
