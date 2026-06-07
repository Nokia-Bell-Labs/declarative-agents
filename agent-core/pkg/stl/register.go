// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import "gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"

// RegisterFileTools registers read, write, edit, find, and list_files
// with the given registry, all scoped to root.
func RegisterFileTools(reg *core.Registry, root string) {
	reg.Register(ReadToolSpec(), &ReadBuilder{Root: root})
	reg.Register(WriteToolSpec(), &WriteBuilder{Root: root})
	reg.Register(EditToolSpec(), &EditBuilder{Root: root})
	reg.Register(FindToolSpec(), &FindBuilder{Root: root})
	reg.Register(ListFilesToolSpec(), &ListFilesBuilder{Root: root})
}

// RegisterBuildTools registers build, vet, lint, and test with the
// given registry, all scoped to root.
func RegisterBuildTools(reg *core.Registry, root string) {
	reg.Register(BuildToolSpec(), &BuildBuilder{Root: root})
	reg.Register(VetToolSpec(), &VetBuilder{Root: root})
	reg.Register(LintToolSpec(), &LintBuilder{Root: root})
	reg.Register(TestToolSpec(), &TestBuilder{Root: root})
}

// RegisterGitTools registers commit, workspace_status, worktree_add, and
// worktree_remove with the given registry, all scoped to root.
func RegisterGitTools(reg *core.Registry, root string) {
	reg.Register(CommitToolSpec(), &CommitBuilder{Root: root})
	reg.Register(WorkspaceStatusToolSpec(), &WorkspaceStatusBuilder{Root: root})
	reg.Register(WorktreeAddToolSpec(), &WorktreeAddBuilder{Root: root})
	reg.Register(WorktreeRemoveToolSpec(), &WorktreeRemoveBuilder{Root: root})
}

// RegisterIssueTools registers issue_create, issue_claim, issue_close,
// and issue_list with the given registry, all scoped to root.
func RegisterIssueTools(reg *core.Registry, root string) {
	reg.Register(IssueCreateToolSpec(), &IssueCreateBuilder{Root: root})
	reg.Register(IssueClaimToolSpec(), &IssueClaimBuilder{Root: root})
	reg.Register(IssueCloseToolSpec(), &IssueCloseBuilder{Root: root})
	reg.Register(IssueListToolSpec(), &IssueListBuilder{Root: root})
}

// RegisterAll registers all standard tools (file + build + git + issue)
// with the given registry, all scoped to root.
func RegisterAll(reg *core.Registry, root string) {
	RegisterFileTools(reg, root)
	RegisterBuildTools(reg, root)
	RegisterGitTools(reg, root)
	RegisterIssueTools(reg, root)
}
