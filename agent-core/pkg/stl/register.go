// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"embed"
	"fmt"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
)

//go:embed tools.yaml
var defaultToolsFS embed.FS

// DefaultToolDefs returns the built-in YAML tool definitions shipped
// with agent-core. These cover build, git, and issue tools.
func DefaultToolDefs() ([]ToolDef, error) {
	data, err := defaultToolsFS.ReadFile("tools.yaml")
	if err != nil {
		return nil, fmt.Errorf("read embedded tools.yaml: %w", err)
	}
	return ParseToolDefs(data)
}

// RegisterFileTools registers read, write, edit, find, and list_files
// with the given registry, all scoped to root. These require Go
// implementations (not CLI wrappers) so they remain as Go code.
func RegisterFileTools(reg *core.Registry, root string) {
	reg.Register(ReadToolSpec(), &ReadBuilder{Root: root})
	reg.Register(WriteToolSpec(), &WriteBuilder{Root: root})
	reg.Register(EditToolSpec(), &EditBuilder{Root: root})
	reg.Register(FindToolSpec(), &FindBuilder{Root: root})
	reg.Register(ListFilesToolSpec(), &ListFilesBuilder{Root: root})
}

// RegisterExecTools registers all YAML-defined tools (build, git, issue)
// from the embedded tools.yaml. Use RegisterExecToolsFrom to load from
// a custom YAML file, or MergeToolDefs to overlay project-specific defs.
func RegisterExecTools(reg *core.Registry, root string) error {
	defs, err := DefaultToolDefs()
	if err != nil {
		return err
	}
	RegisterToolDefs(reg, root, defs)
	return nil
}

// RegisterExecToolsFrom loads tool definitions from a YAML file and
// registers them. Definitions from the file override built-in defaults
// with the same name.
func RegisterExecToolsFrom(reg *core.Registry, root, yamlPath string) error {
	defaults, err := DefaultToolDefs()
	if err != nil {
		return err
	}
	overrides, err := LoadToolDefs(yamlPath)
	if err != nil {
		return err
	}
	merged := MergeToolDefs(defaults, overrides)
	RegisterToolDefs(reg, root, merged)
	return nil
}

// RegisterAll registers all standard tools: file tools (Go) + exec
// tools (YAML). This is the recommended entry point for agents.
func RegisterAll(reg *core.Registry, root string) error {
	RegisterFileTools(reg, root)
	return RegisterExecTools(reg, root)
}

// --- Legacy Go-based registration (deprecated, use RegisterAll) ---
// These remain for backward compatibility. New code should use
// RegisterAll or RegisterExecTools which load from YAML.

// RegisterBuildTools registers build, vet, lint, and test with the
// given registry, all scoped to root.
//
// Deprecated: use RegisterExecTools or RegisterAll instead.
func RegisterBuildTools(reg *core.Registry, root string) {
	reg.Register(BuildToolSpec(), &BuildBuilder{Root: root})
	reg.Register(VetToolSpec(), &VetBuilder{Root: root})
	reg.Register(LintToolSpec(), &LintBuilder{Root: root})
	reg.Register(TestToolSpec(), &TestBuilder{Root: root})
}

// RegisterGitTools registers commit, workspace_status, worktree_add, and
// worktree_remove with the given registry, all scoped to root.
//
// Deprecated: use RegisterExecTools or RegisterAll instead.
func RegisterGitTools(reg *core.Registry, root string) {
	reg.Register(CommitToolSpec(), &CommitBuilder{Root: root})
	reg.Register(WorkspaceStatusToolSpec(), &WorkspaceStatusBuilder{Root: root})
	reg.Register(WorktreeAddToolSpec(), &WorktreeAddBuilder{Root: root})
	reg.Register(WorktreeRemoveToolSpec(), &WorktreeRemoveBuilder{Root: root})
}

// RegisterIssueTools registers issue_create, issue_claim, issue_close,
// and issue_list with the given registry, all scoped to root.
//
// Deprecated: use RegisterExecTools or RegisterAll instead.
func RegisterIssueTools(reg *core.Registry, root string) {
	reg.Register(IssueCreateToolSpec(), &IssueCreateBuilder{Root: root})
	reg.Register(IssueClaimToolSpec(), &IssueClaimBuilder{Root: root})
	reg.Register(IssueCloseToolSpec(), &IssueCloseBuilder{Root: root})
	reg.Register(IssueListToolSpec(), &IssueListBuilder{Root: root})
}
