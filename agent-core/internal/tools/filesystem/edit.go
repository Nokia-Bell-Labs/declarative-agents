// Copyright (c) 2026 Nokia. All rights reserved.

package filesystem

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/observability/monitor"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
)

const editErrorMaxLines = 100

type editCmd struct {
	root        string
	path        string
	oldString   string
	newString   string
	snapshot    fileSnapshot
	hasSnapshot bool
	recorder    monitor.ToolMetricsRecorder
}

func (e *editCmd) Name() string { return "edit" }
func (e *editCmd) Undo() core.Result {
	return undoFileSnapshot(e.Name(), e.root, e.snapshot, e.hasSnapshot)
}
func (e *editCmd) UndoMemento() (core.UndoMemento, error) {
	return fileWorkspaceMemento(e.Name(), e.snapshot, e.hasSnapshot)
}

func (e *editCmd) Execute() core.Result {
	resolved, err := ValidatePath(e.root, e.path)
	if err != nil {
		return toolFailed("edit", fmt.Sprintf("path rejected: %s", err))
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		return editReadError(e.path, err)
	}
	content := string(data)
	count := strings.Count(content, e.oldString)
	relPath := RelPath(e.root, resolved)
	if count != 1 {
		return editMatchFailure(relPath, content, count)
	}
	return e.apply(resolved, relPath, content)
}

func editReadError(path string, err error) core.Result {
	if os.IsNotExist(err) {
		return toolFailed("edit", fmt.Sprintf("file not found: %s", path))
	}
	return commandError("edit", fmt.Errorf("edit read %s: %w", path, err))
}

func editMatchFailure(relPath, content string, count int) core.Result {
	if count == 0 {
		return core.Result{
			Output:      fmt.Sprintf("search text not found in %s\n\nCurrent file contents:\n%s", relPath, fileSnippet(content, editErrorMaxLines)),
			Signal:      core.ToolFailed,
			CommandName: "edit",
			Metrics:     &core.ToolMetrics{Total: 1, Passed: 0, Failed: 1},
		}
	}
	return core.Result{
		Output:      fmt.Sprintf("ambiguous match: %d occurrences found in %s", count, relPath),
		Signal:      core.ToolFailed,
		CommandName: "edit",
		Metrics:     &core.ToolMetrics{Total: 1, Passed: 0, Failed: 1},
	}
}

func (e *editCmd) apply(resolved, relPath, content string) core.Result {
	snapshot, err := snapshotFile(e.root, resolved)
	if err != nil {
		return commandError("edit", fmt.Errorf("edit snapshot %s: %w", e.path, err))
	}
	replaced := strings.Replace(content, e.oldString, e.newString, 1)
	if err := os.WriteFile(resolved, []byte(replaced), 0o644); err != nil {
		return commandError("edit", fmt.Errorf("edit write %s: %w", e.path, err))
	}
	e.snapshot = snapshot
	e.hasSnapshot = true
	e.recordFilesystemMetric("filesystem.bytes_changed", float64(bytesChanged(e.oldString, e.newString)), "By", "Byte delta for one file edit.")
	return core.Result{
		Output:      fmt.Sprintf("replacement applied in %s", relPath),
		Signal:      core.EditDone,
		CommandName: "edit",
		Metrics:     &core.ToolMetrics{Total: 1, Passed: 1, Failed: 0},
	}
}

// EditBuilder constructs edit commands.
type EditBuilder struct {
	Root string
}

func (b *EditBuilder) Build(res core.Result) core.Command {
	p := extractStringParam(res.Output, "path")
	o := extractStringParam(res.Output, "old_string")
	n := extractStringParam(res.Output, "new_string")
	if p == "" {
		return missingParam("edit", "path")
	}
	if o == "" {
		return missingParam("edit", "old_string")
	}
	if n == "" {
		return missingParam("edit", "new_string")
	}
	return &editCmd{root: b.Root, path: p, oldString: o, newString: n}
}

// EditToolSpec returns the ToolSpec for the edit tool.
func EditToolSpec() core.ToolSpec {
	return core.ToolSpec{
		Name:        "edit",
		Description: "Replace the first occurrence of an exact string in a file. Use read first to see the current content, then provide the exact old_string to match.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"File path relative to workspace root"},"old_string":{"type":"string","description":"Exact text to find"},"new_string":{"type":"string","description":"Replacement text"}},"required":["path","old_string","new_string"]}`),
		Visibility:  core.External,
	}
}

func fileSnippet(content string, maxLines int) string {
	lines := strings.Split(content, "\n")
	end := len(lines)
	truncated := false
	if end > maxLines {
		end = maxLines
		truncated = true
	}
	width := len(fmt.Sprintf("%d", end))
	var sb strings.Builder
	for i := 0; i < end; i++ {
		fmt.Fprintf(&sb, "%*d|%s\n", width, i+1, lines[i])
	}
	if truncated {
		fmt.Fprintf(&sb, "\n... %d lines omitted", len(lines)-end)
	}
	return strings.TrimRight(sb.String(), "\n")
}
