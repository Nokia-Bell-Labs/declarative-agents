// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
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

func TestRegisterBuildTools(t *testing.T) {
	reg := core.NewRegistry()
	RegisterBuildTools(reg, "/tmp")

	expected := []string{"build", "lint", "test", "vet"}
	assert.Equal(t, expected, reg.ExternalToolNames())

	for _, name := range expected {
		_, ok := reg.Resolve(name)
		assert.True(t, ok, "should resolve %s", name)
	}
}

func TestRegisterGitTools(t *testing.T) {
	reg := core.NewRegistry()
	RegisterGitTools(reg, "/tmp")

	expected := []string{"commit", "workspace_status", "worktree_add", "worktree_remove"}
	assert.Equal(t, expected, reg.ExternalToolNames())

	for _, name := range expected {
		_, ok := reg.Resolve(name)
		assert.True(t, ok, "should resolve %s", name)
	}
}

func TestRegisterIssueTools(t *testing.T) {
	reg := core.NewRegistry()
	RegisterIssueTools(reg, "/tmp")

	expected := []string{"issue_claim", "issue_close", "issue_create", "issue_list"}
	assert.Equal(t, expected, reg.ExternalToolNames())

	for _, name := range expected {
		_, ok := reg.Resolve(name)
		assert.True(t, ok, "should resolve %s", name)
	}
}

func TestRegisterAll(t *testing.T) {
	reg := core.NewRegistry()
	RegisterAll(reg, "/tmp")

	names := reg.AllToolNames()
	assert.Len(t, names, 17)
	// file tools
	assert.Contains(t, names, "read")
	assert.Contains(t, names, "write")
	assert.Contains(t, names, "edit")
	assert.Contains(t, names, "find")
	assert.Contains(t, names, "list_files")
	// build tools
	assert.Contains(t, names, "build")
	assert.Contains(t, names, "vet")
	assert.Contains(t, names, "lint")
	assert.Contains(t, names, "test")
	// git tools
	assert.Contains(t, names, "commit")
	assert.Contains(t, names, "workspace_status")
	assert.Contains(t, names, "worktree_add")
	assert.Contains(t, names, "worktree_remove")
	// issue tools
	assert.Contains(t, names, "issue_create")
	assert.Contains(t, names, "issue_claim")
	assert.Contains(t, names, "issue_close")
	assert.Contains(t, names, "issue_list")
}

func TestToolSpecs_HaveDescriptions(t *testing.T) {
	specs := []core.ToolSpec{
		ReadToolSpec(), WriteToolSpec(), EditToolSpec(),
		FindToolSpec(), ListFilesToolSpec(),
		BuildToolSpec(), VetToolSpec(), LintToolSpec(), TestToolSpec(),
		CommitToolSpec(), WorkspaceStatusToolSpec(),
		WorktreeAddToolSpec(), WorktreeRemoveToolSpec(),
		IssueCreateToolSpec(), IssueClaimToolSpec(),
		IssueCloseToolSpec(), IssueListToolSpec(),
	}

	for _, s := range specs {
		assert.NotEmpty(t, s.Name, "spec should have a name")
		assert.NotEmpty(t, s.Description, "spec %s should have a description", s.Name)
		assert.NotEmpty(t, s.InputSchema, "spec %s should have an input schema", s.Name)
		assert.Equal(t, core.External, s.Visibility, "spec %s should be external", s.Name)
	}
}
