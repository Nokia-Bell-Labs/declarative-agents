// Copyright (c) 2026 Nokia. All rights reserved.

// Deprecated: This file contains compound Go tool implementations that
// are superseded by atomic YAML-defined tools in tools.yaml. Use
// RegisterExecTools or RegisterAll to register the YAML versions. These
// types remain for backward compatibility and will be removed in a
// future release.
package stl

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
)

// --- commit tool (DEPRECATED: use atomic stage_all + commit from tools.yaml) ---

type commitCmd struct {
	root    string
	message string
}

func (c *commitCmd) Name() string { return "commit" }

func (c *commitCmd) Execute() core.Result {
	if err := verifyGitDir(c.root); err != nil {
		return core.Result{
			Output:      err.Error(),
			Signal:      core.ToolFailed,
			CommandName: "commit",
		}
	}

	if _, err := runGitCmd(c.root, "add", "-A"); err != nil {
		return core.Result{
			Output:      fmt.Sprintf("git add failed: %s", err),
			Signal:      core.ToolFailed,
			CommandName: "commit",
		}
	}

	status, err := runGitCmd(c.root, "status", "--porcelain")
	if err != nil {
		return core.Result{
			Output:      fmt.Sprintf("git status failed: %s", err),
			Signal:      core.ToolFailed,
			CommandName: "commit",
		}
	}
	if strings.TrimSpace(status) == "" {
		return core.Result{
			Output:      "nothing to commit, working tree clean",
			Signal:      core.ToolDone,
			CommandName: "commit",
		}
	}

	out, err := runGitCmd(c.root, "commit", "-m", c.message)
	if err != nil {
		return core.Result{
			Output:      fmt.Sprintf("git commit failed: %s", err),
			Signal:      core.ToolFailed,
			CommandName: "commit",
		}
	}

	hash, _ := runGitCmd(c.root, "rev-parse", "--short", "HEAD")
	return core.Result{
		Output:      fmt.Sprintf("committed %s: %s", strings.TrimSpace(hash), strings.TrimSpace(out)),
		Signal:      core.ToolDone,
		CommandName: "commit",
	}
}

// CommitBuilder constructs commit commands.
//
// Deprecated: Use atomic stage_all + commit tools from tools.yaml instead.
type CommitBuilder struct {
	Root string
}

func (b *CommitBuilder) Build(res core.Result) core.Command {
	msg := ExtractStringParam(res.Output, "message")
	if msg == "" {
		return &FailedParamCmd{ToolName: "commit", Missing: "message"}
	}
	return &commitCmd{root: b.Root, message: msg}
}

// CommitToolSpec returns the ToolSpec for the commit tool.
//
// Deprecated: Use atomic stage_all + commit tools from tools.yaml instead.
func CommitToolSpec() core.ToolSpec {
	return core.ToolSpec{
		Name:        "commit",
		Description: "Stage all changes and create a git commit. Requires a message. Returns the commit hash. Reports 'nothing to commit' if the working tree is clean. Side effects: stages all files (git add -A) and creates a commit.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"message":{"type":"string","description":"Commit message"}},"required":["message"]}`),
		Visibility:  core.External,
	}
}

// --- workspace_status tool ---

type workspaceStatusCmd struct {
	root string
}

func (w *workspaceStatusCmd) Name() string { return "workspace_status" }

func (w *workspaceStatusCmd) Execute() core.Result {
	if err := verifyGitDir(w.root); err != nil {
		return core.Result{
			Output:      err.Error(),
			Signal:      core.ToolFailed,
			CommandName: "workspace_status",
		}
	}

	branch, _ := runGitCmd(w.root, "branch", "--show-current")
	status, err := runGitCmd(w.root, "status", "--porcelain")
	if err != nil {
		return core.Result{
			Output:      fmt.Sprintf("git status failed: %s", err),
			Signal:      core.ToolFailed,
			CommandName: "workspace_status",
		}
	}

	status = strings.TrimSpace(status)
	branch = strings.TrimSpace(branch)

	var sb strings.Builder
	if status == "" {
		fmt.Fprintf(&sb, "clean\nbranch: %s", branch)
	} else {
		fmt.Fprintf(&sb, "dirty\nbranch: %s\n%s", branch, status)
	}

	return core.Result{
		Output:      sb.String(),
		Signal:      core.ToolDone,
		CommandName: "workspace_status",
	}
}

// WorkspaceStatusBuilder constructs workspace_status commands.
//
// Deprecated: Use workspace_status from tools.yaml instead.
type WorkspaceStatusBuilder struct {
	Root string
}

func (b *WorkspaceStatusBuilder) Build(_ core.Result) core.Command {
	return &workspaceStatusCmd{root: b.Root}
}

// WorkspaceStatusToolSpec returns the ToolSpec for the workspace_status tool.
//
// Deprecated: Use workspace_status from tools.yaml instead.
func WorkspaceStatusToolSpec() core.ToolSpec {
	return core.ToolSpec{
		Name:        "workspace_status",
		Description: "Report git workspace state: clean or dirty, current branch, and changed files with status codes (M/A/D/??). Read-only, no side effects.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
		Visibility:  core.External,
	}
}

// --- worktree_add tool ---

type worktreeAddCmd struct {
	repoDir string
	id      string
	prefix  string
}

func (w *worktreeAddCmd) Name() string { return "worktree_add" }

func (w *worktreeAddCmd) Execute() core.Result {
	if err := verifyGitDir(w.repoDir); err != nil {
		return core.Result{
			Output:      err.Error(),
			Signal:      core.ToolFailed,
			CommandName: "worktree_add",
		}
	}

	branch := w.prefix + w.id
	dir := filepath.Clean(w.repoDir) + "-" + w.id

	exists, _ := runGitCmd(w.repoDir, "branch", "--list", branch)
	if strings.TrimSpace(exists) != "" {
		return core.Result{
			Output:      fmt.Sprintf("branch %s already exists", branch),
			Signal:      core.ToolFailed,
			CommandName: "worktree_add",
		}
	}

	if _, err := os.Stat(dir); err == nil {
		return core.Result{
			Output:      fmt.Sprintf("worktree directory already exists: %s", dir),
			Signal:      core.ToolFailed,
			CommandName: "worktree_add",
		}
	}

	_, err := runGitCmd(w.repoDir, "worktree", "add", "-b", branch, dir)
	if err != nil {
		return core.Result{
			Output:      fmt.Sprintf("git worktree add failed: %s", err),
			Signal:      core.ToolFailed,
			CommandName: "worktree_add",
		}
	}

	return core.Result{
		Output:      fmt.Sprintf("worktree: %s\nbranch: %s", dir, branch),
		Signal:      core.ToolDone,
		CommandName: "worktree_add",
	}
}

// WorktreeAddBuilder constructs worktree_add commands.
//
// Deprecated: Use atomic worktree_add from tools.yaml instead.
type WorktreeAddBuilder struct {
	Root string
}

func (b *WorktreeAddBuilder) Build(res core.Result) core.Command {
	id := ExtractStringParam(res.Output, "id")
	if id == "" {
		return &FailedParamCmd{ToolName: "worktree_add", Missing: "id"}
	}
	prefix := ExtractStringParam(res.Output, "prefix")
	if prefix == "" {
		prefix = "work/"
	}
	return &worktreeAddCmd{repoDir: b.Root, id: id, prefix: prefix}
}

// WorktreeAddToolSpec returns the ToolSpec for the worktree_add tool.
//
// Deprecated: Use atomic worktree_add from tools.yaml instead.
func WorktreeAddToolSpec() core.ToolSpec {
	return core.ToolSpec{
		Name:        "worktree_add",
		Description: "Create an isolated git worktree with a new branch. Returns the worktree path and branch name. Side effects: creates a new branch and a new directory as a sibling of the repo. Reversible via worktree_remove.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string","description":"Task identifier used to derive branch and directory names"},"prefix":{"type":"string","description":"Branch name prefix (default: work/)"}},"required":["id"]}`),
		Visibility:  core.External,
	}
}

// --- worktree_remove tool ---

type worktreeRemoveCmd struct {
	repoDir string
	dir     string
	branch  string
}

func (w *worktreeRemoveCmd) Name() string { return "worktree_remove" }

func (w *worktreeRemoveCmd) Execute() core.Result {
	if err := verifyGitDir(w.repoDir); err != nil {
		return core.Result{
			Output:      err.Error(),
			Signal:      core.ToolFailed,
			CommandName: "worktree_remove",
		}
	}

	if _, err := os.Stat(w.dir); os.IsNotExist(err) {
		return core.Result{
			Output:      fmt.Sprintf("worktree already removed: %s", w.dir),
			Signal:      core.ToolDone,
			CommandName: "worktree_remove",
		}
	}

	if _, err := runGitCmd(w.repoDir, "worktree", "remove", "--force", w.dir); err != nil {
		return core.Result{
			Output:      fmt.Sprintf("git worktree remove failed: %s", err),
			Signal:      core.ToolFailed,
			CommandName: "worktree_remove",
		}
	}

	// Best-effort branch delete
	if w.branch != "" {
		_, _ = runGitCmd(w.repoDir, "branch", "-D", w.branch)
	}

	return core.Result{
		Output:      fmt.Sprintf("removed worktree %s and branch %s", w.dir, w.branch),
		Signal:      core.ToolDone,
		CommandName: "worktree_remove",
	}
}

// WorktreeRemoveBuilder constructs worktree_remove commands.
//
// Deprecated: Use atomic worktree_remove from tools.yaml instead.
type WorktreeRemoveBuilder struct {
	Root string
}

func (b *WorktreeRemoveBuilder) Build(res core.Result) core.Command {
	dir := ExtractStringParam(res.Output, "dir")
	branch := ExtractStringParam(res.Output, "branch")
	if dir == "" {
		return &FailedParamCmd{ToolName: "worktree_remove", Missing: "dir"}
	}
	if branch == "" {
		return &FailedParamCmd{ToolName: "worktree_remove", Missing: "branch"}
	}
	return &worktreeRemoveCmd{repoDir: b.Root, dir: dir, branch: branch}
}

// WorktreeRemoveToolSpec returns the ToolSpec for the worktree_remove tool.
//
// Deprecated: Use atomic worktree_remove from tools.yaml instead.
func WorktreeRemoveToolSpec() core.ToolSpec {
	return core.ToolSpec{
		Name:        "worktree_remove",
		Description: "Remove a git worktree and delete its branch. Idempotent: succeeds if already removed. Irreversible: uncommitted changes in the worktree are lost. Commit before calling this tool.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"dir":{"type":"string","description":"Worktree directory path"},"branch":{"type":"string","description":"Branch name to delete"}},"required":["dir","branch"]}`),
		Visibility:  core.External,
	}
}

// --- helpers ---

func verifyGitDir(dir string) error {
	gitPath := filepath.Join(dir, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("not a git repository: %s", dir)
		}
		return fmt.Errorf("checking git repo %s: %v", dir, err)
	}
	// .git can be a file (worktree) or directory (main repo)
	_ = info
	return nil
}

func runGitCmd(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		se := strings.TrimSpace(stderr.String())
		if se != "" {
			return "", fmt.Errorf("%s", se)
		}
		return "", err
	}
	return string(out), nil
}
