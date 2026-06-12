// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
)

func TestRegisterFileTools(t *testing.T) {
	reg := core.NewRegistry()
	RegisterFileTools(reg, "/tmp")

	expected := []string{"edit", "find", "list_files", "read", "write"}
	assert.Equal(t, expected, reg.ExternalToolNames())

	for _, name := range expected {
		_, ok := reg.Resolve(name)
		assert.True(t, ok, "should resolve %s", name)
	}
}

func TestDefaultToolDefs(t *testing.T) {
	defs, err := DefaultToolDefs()
	require.NoError(t, err)

	names := make(map[string]bool)
	for _, d := range defs {
		names[d.Name] = true
	}

	for _, expected := range []string{
		"build", "vet", "lint", "test",
		"stage_all", "workspace_status", "commit", "rev_parse",
		"branch_create", "branch_delete", "branch_list", "branch_current",
		"worktree_add", "worktree_remove", "worktree_list",
		"diff_stat", "log_oneline",
		"issue_create", "issue_close", "issue_list", "issue_claim",
	} {
		assert.True(t, names[expected], "missing tool %q", expected)
	}
}

func TestRegisterExecTools(t *testing.T) {
	reg := core.NewRegistry()
	err := RegisterExecTools(reg, "/tmp")
	require.NoError(t, err)

	names := reg.ExternalToolNames()
	assert.Contains(t, names, "build")
	assert.Contains(t, names, "commit")
	assert.Contains(t, names, "stage_all")
	assert.Contains(t, names, "issue_create")
	assert.Contains(t, names, "worktree_add")
	assert.Contains(t, names, "branch_create")
	assert.Contains(t, names, "log_oneline")

	for _, name := range names {
		_, ok := reg.Resolve(name)
		assert.True(t, ok, "should resolve %s", name)
	}
}

func TestRegisterAll(t *testing.T) {
	reg := core.NewRegistry()
	err := RegisterAll(reg, "/tmp")
	require.NoError(t, err)

	names := reg.AllToolNames()
	// 5 file tools + 21 YAML-defined exec tools = 26
	assert.Len(t, names, 26)

	// file tools (Go)
	for _, name := range []string{"read", "write", "edit", "find", "list_files"} {
		assert.Contains(t, names, name)
	}
	// build tools (YAML)
	for _, name := range []string{"build", "vet", "lint", "test"} {
		assert.Contains(t, names, name)
	}
	// git tools (YAML, atomic)
	for _, name := range []string{
		"stage_all", "commit", "rev_parse", "workspace_status",
		"branch_create", "branch_delete", "branch_list", "branch_current",
		"worktree_add", "worktree_remove", "worktree_list",
		"diff_stat", "log_oneline",
	} {
		assert.Contains(t, names, name)
	}
	// issue tools (YAML)
	for _, name := range []string{"issue_create", "issue_claim", "issue_close", "issue_list"} {
		assert.Contains(t, names, name)
	}
}

func TestToolSpecs_HaveDescriptions(t *testing.T) {
	specs := []core.ToolSpec{
		ReadToolSpec(), WriteToolSpec(), EditToolSpec(),
		FindToolSpec(), ListFilesToolSpec(),
	}

	for _, s := range specs {
		assert.NotEmpty(t, s.Name, "spec should have a name")
		assert.NotEmpty(t, s.Description, "spec %s should have a description", s.Name)
		assert.NotEmpty(t, s.InputSchema, "spec %s should have an input schema", s.Name)
		assert.Equal(t, core.External, s.Visibility, "spec %s should be external", s.Name)
	}
}

func TestYAMLToolSpecs_HaveDescriptions(t *testing.T) {
	defs, err := DefaultToolDefs()
	require.NoError(t, err)

	for _, d := range defs {
		spec := d.ToToolSpec()
		assert.NotEmpty(t, spec.Name, "spec should have a name")
		assert.NotEmpty(t, spec.Description, "spec %s should have a description", spec.Name)
		assert.NotEmpty(t, spec.InputSchema, "spec %s should have an input schema", spec.Name)
	}
}
