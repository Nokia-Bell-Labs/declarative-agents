// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
)

const testToolsYAML = `
tools:
  - name: greet
    binary: echo
    args: [hello]
    description: "Say hello"
    params:
      - name: name
        flag: --name
        required: true
        description: "Name to greet"
      - name: loud
        flag: --loud
        bool_flag: true
        description: "Shout"
  - name: list_dir
    binary: ls
    args: [-la]
    description: "List directory"
    params:
      - name: path
        positional: true
        default: "."
        description: "Directory to list"
`

func TestParseToolDefs(t *testing.T) {
	defs, err := ParseToolDefs([]byte(testToolsYAML))
	require.NoError(t, err)
	assert.Len(t, defs, 2)

	assert.Equal(t, "greet", defs[0].Name)
	assert.Equal(t, "echo", defs[0].Binary)
	assert.Equal(t, []string{"hello"}, defs[0].Args)
	assert.Len(t, defs[0].Params, 2)
	assert.True(t, defs[0].Params[0].Required)
	assert.True(t, defs[0].Params[1].BoolFlag)

	assert.Equal(t, "list_dir", defs[1].Name)
	assert.True(t, defs[1].Params[0].Positional)
	assert.Equal(t, ".", defs[1].Params[0].Default)
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
		SideEffects: "produces binary",
		Params: []ParamDef{
			{Name: "pkg", Flag: "--pkg", Required: true, Desc: "Package"},
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
	assert.Contains(t, props, "pkg")
	req := schema["required"].([]interface{})
	assert.Contains(t, req, "pkg")
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

func TestExecBuilder_MissingRequired(t *testing.T) {
	td := ToolDef{
		Name:   "greet",
		Binary: "echo",
		Params: []ParamDef{
			{Name: "name", Flag: "--name", Required: true},
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
		Params: []ParamDef{
			{Name: "path", Positional: true, Default: "."},
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
		Params: []ParamDef{
			{Name: "package", Positional: true},
			{Name: "verbose", Flag: "-v", BoolFlag: true},
		},
	}

	cmd := &ExecCmd{
		def:    def,
		root:   "/tmp",
		params: map[string]string{"package": "./pkg/...", "verbose": "true"},
	}

	args := cmd.buildArgs()
	assert.Equal(t, []string{"test", "-count=1", "./pkg/...", "-v"}, args)
}

func TestExecCmd_BuildArgs_FlagParams(t *testing.T) {
	def := ToolDef{
		Name:   "create",
		Binary: "bd",
		Args:   []string{"create", "--json"},
		Params: []ParamDef{
			{Name: "title", Flag: "--title"},
			{Name: "body", Flag: "--body"},
		},
	}

	cmd := &ExecCmd{
		def:    def,
		root:   "/tmp",
		params: map[string]string{"title": "fix bug"},
	}

	args := cmd.buildArgs()
	assert.Equal(t, []string{"create", "--json", "--title", "fix bug"}, args)
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
		{Name: "build", Binary: "go", Args: []string{"build"}},
		{Name: "test", Binary: "go", Args: []string{"test"}},
	}
	override := []ToolDef{
		{Name: "build", Binary: "go", Args: []string{"build", "-race"}},
		{Name: "lint", Binary: "golangci-lint"},
	}

	merged := MergeToolDefs(base, override)
	assert.Len(t, merged, 3)

	assert.Equal(t, "build", merged[0].Name)
	assert.Equal(t, []string{"build", "-race"}, merged[0].Args)
	assert.Equal(t, "test", merged[1].Name)
	assert.Equal(t, "lint", merged[2].Name)
}

func TestLoadDefaultToolDefs(t *testing.T) {
	defs, err := LoadToolDefs("tools.yaml")
	require.NoError(t, err)
	assert.True(t, len(defs) >= 9, "expected at least 9 default tool defs, got %d", len(defs))

	names := make(map[string]bool)
	for _, d := range defs {
		names[d.Name] = true
	}
	for _, expected := range []string{"build", "vet", "lint", "test",
		"workspace_status", "issue_create", "issue_close", "issue_list", "issue_claim"} {
		assert.True(t, names[expected], "missing tool %q", expected)
	}
}
