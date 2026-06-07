// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
)

const bdBin = "bd"

// --- issue_create tool ---

type issueCreateCmd struct {
	root     string
	title    string
	body     string
	issType  string
	priority string
	deps     string
}

func (c *issueCreateCmd) Name() string { return "issue_create" }

func (c *issueCreateCmd) Execute() core.Result {
	args := []string{"create", "--title", c.title, "--json"}

	if c.body != "" {
		args = append(args, "--body", c.body)
	}
	if c.issType != "" {
		args = append(args, "-t", c.issType)
	}
	if c.priority != "" {
		args = append(args, "-p", c.priority)
	}
	if c.deps != "" {
		args = append(args, "--deps", c.deps)
	}

	out, err := runBdCmd(c.root, args...)
	if err != nil {
		return core.Result{
			Output:      fmt.Sprintf("issue_create failed: %s", err),
			Signal:      core.ToolFailed,
			CommandName: "issue_create",
		}
	}

	var result struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		return core.Result{
			Output:      fmt.Sprintf("issue_create: failed to parse bd output: %s", err),
			Signal:      core.ToolFailed,
			CommandName: "issue_create",
		}
	}

	return core.Result{
		Output:      fmt.Sprintf("created issue %s: %s", result.ID, result.Title),
		Signal:      core.ToolDone,
		CommandName: "issue_create",
	}
}

// IssueCreateBuilder constructs issue_create commands.
type IssueCreateBuilder struct {
	Root string
}

func (b *IssueCreateBuilder) Build(res core.Result) core.Command {
	title := ExtractStringParam(res.Output, "title")
	if title == "" {
		return &FailedParamCmd{ToolName: "issue_create", Missing: "title"}
	}
	return &issueCreateCmd{
		root:     b.Root,
		title:    title,
		body:     ExtractStringParam(res.Output, "body"),
		issType:  ExtractStringParam(res.Output, "type"),
		priority: ExtractStringParam(res.Output, "priority"),
		deps:     ExtractStringParam(res.Output, "deps"),
	}
}

// IssueCreateToolSpec returns the ToolSpec for the issue_create tool.
func IssueCreateToolSpec() core.ToolSpec {
	return core.ToolSpec{
		Name:        "issue_create",
		Description: "Create a new beads issue. Returns the issue ID. Side effects: writes to .beads/issues.jsonl and .beads/interactions.jsonl. Compensatable: issue can be closed but not deleted.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"title":{"type":"string","description":"Issue title"},"body":{"type":"string","description":"Issue body/description"},"type":{"type":"string","description":"Issue type (default: task)"},"priority":{"type":"string","description":"Priority level (default: P2)"},"deps":{"type":"string","description":"Comma-separated dependency issue IDs"}},"required":["title"]}`),
		Visibility:  core.External,
	}
}

// --- issue_claim tool ---

type issueClaimCmd struct {
	root string
	id   string
}

func (c *issueClaimCmd) Name() string { return "issue_claim" }

func (c *issueClaimCmd) Execute() core.Result {
	out, err := runBdCmd(c.root, "update", c.id, "--claim")
	if err != nil {
		return core.Result{
			Output:      fmt.Sprintf("issue_claim failed: %s", err),
			Signal:      core.ToolFailed,
			CommandName: "issue_claim",
		}
	}

	return core.Result{
		Output:      strings.TrimSpace(out),
		Signal:      core.ToolDone,
		CommandName: "issue_claim",
	}
}

// IssueClaimBuilder constructs issue_claim commands.
type IssueClaimBuilder struct {
	Root string
}

func (b *IssueClaimBuilder) Build(res core.Result) core.Command {
	id := ExtractStringParam(res.Output, "id")
	if id == "" {
		return &FailedParamCmd{ToolName: "issue_claim", Missing: "id"}
	}
	return &issueClaimCmd{root: b.Root, id: id}
}

// IssueClaimToolSpec returns the ToolSpec for the issue_claim tool.
func IssueClaimToolSpec() core.ToolSpec {
	return core.ToolSpec{
		Name:        "issue_claim",
		Description: "Claim an issue: set status to in_progress and assign to current user. Idempotent if already claimed by you. Side effects: modifies issue status in .beads/issues.jsonl. Compensatable: status can be reset via issue update.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string","description":"Issue ID to claim"}},"required":["id"]}`),
		Visibility:  core.External,
	}
}

// --- issue_close tool ---

type issueCloseCmd struct {
	root string
	id   string
}

func (c *issueCloseCmd) Name() string { return "issue_close" }

func (c *issueCloseCmd) Execute() core.Result {
	out, err := runBdCmd(c.root, "close", c.id)
	if err != nil {
		return core.Result{
			Output:      fmt.Sprintf("issue_close failed: %s", err),
			Signal:      core.ToolFailed,
			CommandName: "issue_close",
		}
	}

	return core.Result{
		Output:      strings.TrimSpace(out),
		Signal:      core.ToolDone,
		CommandName: "issue_close",
	}
}

// IssueCloseBuilder constructs issue_close commands.
type IssueCloseBuilder struct {
	Root string
}

func (b *IssueCloseBuilder) Build(res core.Result) core.Command {
	id := ExtractStringParam(res.Output, "id")
	if id == "" {
		return &FailedParamCmd{ToolName: "issue_close", Missing: "id"}
	}
	return &issueCloseCmd{root: b.Root, id: id}
}

// IssueCloseToolSpec returns the ToolSpec for the issue_close tool.
func IssueCloseToolSpec() core.ToolSpec {
	return core.ToolSpec{
		Name:        "issue_close",
		Description: "Close an issue by ID. Side effects: sets issue status to closed in .beads/issues.jsonl. Compensatable: issue can be reopened.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string","description":"Issue ID to close"}},"required":["id"]}`),
		Visibility:  core.External,
	}
}

// --- issue_list tool ---

type issueListCmd struct {
	root   string
	status string
}

func (c *issueListCmd) Name() string { return "issue_list" }

func (c *issueListCmd) Execute() core.Result {
	args := []string{"list"}
	if c.status == "all" {
		args = append(args, "--all")
	}

	out, err := runBdCmd(c.root, args...)
	if err != nil {
		return core.Result{
			Output:      fmt.Sprintf("issue_list failed: %s", err),
			Signal:      core.ToolFailed,
			CommandName: "issue_list",
		}
	}

	trimmed := strings.TrimSpace(out)
	if trimmed == "" || strings.Contains(trimmed, "No issues found") {
		return core.Result{
			Output:      "No open issues.",
			Signal:      core.ToolDone,
			CommandName: "issue_list",
		}
	}

	return core.Result{
		Output:      trimmed,
		Signal:      core.ToolDone,
		CommandName: "issue_list",
	}
}

// IssueListBuilder constructs issue_list commands.
type IssueListBuilder struct {
	Root string
}

func (b *IssueListBuilder) Build(res core.Result) core.Command {
	status := ExtractStringParam(res.Output, "status")
	if status == "" {
		status = "open"
	}
	return &issueListCmd{root: b.Root, status: status}
}

// IssueListToolSpec returns the ToolSpec for the issue_list tool.
func IssueListToolSpec() core.ToolSpec {
	return core.ToolSpec{
		Name:        "issue_list",
		Description: "List beads issues. Defaults to open issues only. Use status 'all' to include closed issues. Read-only, no side effects.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"status":{"type":"string","description":"Filter: 'open' (default) or 'all'"}}}`),
		Visibility:  core.External,
	}
}

// --- helpers ---

func runBdCmd(dir string, args ...string) (string, error) {
	cmd := exec.Command(bdBin, args...)
	if dir != "" {
		cmd.Dir = dir
	}
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
