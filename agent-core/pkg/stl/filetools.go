// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
)

// --- read tool ---

type readCmd struct {
	root      string
	path      string
	startLine int
	endLine   int
}

func (r *readCmd) Name() string { return "read" }

func (r *readCmd) Execute() core.Result {
	resolved, err := ValidatePath(r.root, r.path)
	if err != nil {
		return core.Result{
			Output:      fmt.Sprintf("path rejected: %s", err),
			Signal:      core.ToolFailed,
			CommandName: "read",
		}
	}

	info, statErr := os.Stat(resolved)
	if statErr == nil && info.IsDir() {
		entries, _ := os.ReadDir(resolved)
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		return core.Result{
			Output:      fmt.Sprintf("path is a directory, not a file: %s\nContents: %s\nUse read on a specific file.", r.path, strings.Join(names, ", ")),
			Signal:      core.ToolFailed,
			CommandName: "read",
		}
	}

	data, err := os.ReadFile(resolved)
	if err != nil {
		if os.IsNotExist(err) {
			return core.Result{
				Output:      fmt.Sprintf("file not found: %s", r.path),
				Signal:      core.ToolFailed,
				CommandName: "read",
			}
		}
		return core.Result{
			Signal:      core.ToolFailed,
			Output:      fmt.Sprintf("cannot read %s: %s", r.path, err),
			CommandName: "read",
		}
	}

	if IsBinary(data) {
		return core.Result{
			Output:      fmt.Sprintf("file appears to be binary: %s", RelPath(r.root, resolved)),
			Signal:      core.ToolFailed,
			CommandName: "read",
		}
	}

	lines := strings.Split(string(data), "\n")
	start, end := r.startLine, r.endLine
	if start <= 0 {
		start = 1
	}
	if end <= 0 || end > len(lines) {
		end = len(lines)
	}
	if start > len(lines) {
		return core.Result{
			Output:      "",
			Signal:      core.ToolDone,
			CommandName: "read",
		}
	}

	var sb strings.Builder
	width := len(fmt.Sprintf("%d", end))
	for i := start; i <= end; i++ {
		fmt.Fprintf(&sb, "%*d|%s\n", width, i, lines[i-1])
	}

	return core.Result{
		Output:      strings.TrimRight(sb.String(), "\n"),
		Signal:      core.ToolDone,
		CommandName: "read",
	}
}

// IsBinary checks the first 512 bytes for null bytes.
func IsBinary(data []byte) bool {
	check := data
	if len(check) > 512 {
		check = check[:512]
	}
	return bytes.Contains(check, []byte{0})
}

// ReadBuilder constructs read commands.
type ReadBuilder struct {
	Root string
}

func (b *ReadBuilder) Build(res core.Result) core.Command {
	p := ExtractStringParam(res.Output, "path")
	if p == "" {
		return &FailedParamCmd{ToolName: "read", Missing: "path"}
	}
	return &readCmd{
		root:      b.Root,
		path:      p,
		startLine: ExtractIntParam(res.Output, "start_line"),
		endLine:   ExtractIntParam(res.Output, "end_line"),
	}
}

// ReadToolSpec returns the ToolSpec for the read tool.
func ReadToolSpec() core.ToolSpec {
	return core.ToolSpec{
		Name:        "read",
		Description: "Read a single file's contents. Path must point to a file, not a directory. Use find to discover file paths first.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"File path relative to workspace root (must be a file, not a directory)"},"start_line":{"type":"integer","description":"Start line (1-indexed, inclusive)"},"end_line":{"type":"integer","description":"End line (1-indexed, inclusive)"}},"required":["path"]}`),
		Visibility:  core.External,
	}
}

// --- write tool ---

type writeCmd struct {
	root    string
	path    string
	content string
}

func (w *writeCmd) Name() string { return "write" }

func (w *writeCmd) Execute() core.Result {
	resolved, err := ValidatePath(w.root, w.path)
	if err != nil {
		if !strings.Contains(err.Error(), "not found") {
			return core.Result{
				Output:      fmt.Sprintf("path rejected: %s", err),
				Signal:      core.ToolFailed,
				CommandName: "write",
			}
		}
		joined := w.path
		if !filepath.IsAbs(w.path) {
			joined = filepath.Join(w.root, w.path)
		}
		joined = filepath.Clean(joined)
		resolved = joined
	}

	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		return core.Result{
			Signal:      core.CommandError,
			Err:         fmt.Errorf("write mkdir %s: %w", w.path, err),
			CommandName: "write",
		}
	}

	if err := os.WriteFile(resolved, []byte(w.content), 0o644); err != nil {
		return core.Result{
			Signal:      core.CommandError,
			Err:         fmt.Errorf("write %s: %w", w.path, err),
			CommandName: "write",
		}
	}

	relPath := RelPath(w.root, resolved)
	return core.Result{
		Output:      fmt.Sprintf("wrote %d bytes to %s", len(w.content), relPath),
		Signal:      core.ToolDone,
		CommandName: "write",
	}
}

// WriteBuilder constructs write commands.
type WriteBuilder struct {
	Root string
}

func (b *WriteBuilder) Build(res core.Result) core.Command {
	p := ExtractStringParam(res.Output, "path")
	c := ExtractStringParam(res.Output, "content")
	if p == "" {
		return &FailedParamCmd{ToolName: "write", Missing: "path"}
	}
	if c == "" {
		return &FailedParamCmd{ToolName: "write", Missing: "content"}
	}
	return &writeCmd{root: b.Root, path: p, content: c}
}

// WriteToolSpec returns the ToolSpec for the write tool.
func WriteToolSpec() core.ToolSpec {
	return core.ToolSpec{
		Name:        "write",
		Description: "Create or overwrite a file. Provide the complete file content — this replaces the entire file. Parent directories are created automatically.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"File path relative to workspace root"},"content":{"type":"string","description":"Full file content to write"}},"required":["path","content"]}`),
		Visibility:  core.External,
	}
}

// --- edit tool ---

type editCmd struct {
	root      string
	path      string
	oldString string
	newString string
}

func (e *editCmd) Name() string { return "edit" }

func (e *editCmd) Execute() core.Result {
	resolved, err := ValidatePath(e.root, e.path)
	if err != nil {
		return core.Result{
			Output:      fmt.Sprintf("path rejected: %s", err),
			Signal:      core.ToolFailed,
			CommandName: "edit",
		}
	}

	data, err := os.ReadFile(resolved)
	if err != nil {
		if os.IsNotExist(err) {
			return core.Result{
				Output:      fmt.Sprintf("file not found: %s", e.path),
				Signal:      core.ToolFailed,
				CommandName: "edit",
			}
		}
		return core.Result{
			Signal:      core.CommandError,
			Err:         fmt.Errorf("edit read %s: %w", e.path, err),
			CommandName: "edit",
		}
	}

	content := string(data)
	count := strings.Count(content, e.oldString)

	if count == 0 {
		return core.Result{
			Output:      fmt.Sprintf("search text not found in %s", RelPath(e.root, resolved)),
			Signal:      core.ToolFailed,
			CommandName: "edit",
		}
	}

	if count > 1 {
		return core.Result{
			Output:      fmt.Sprintf("ambiguous match: %d occurrences found in %s", count, RelPath(e.root, resolved)),
			Signal:      core.ToolFailed,
			CommandName: "edit",
		}
	}

	replaced := strings.Replace(content, e.oldString, e.newString, 1)
	if err := os.WriteFile(resolved, []byte(replaced), 0o644); err != nil {
		return core.Result{
			Signal:      core.CommandError,
			Err:         fmt.Errorf("edit write %s: %w", e.path, err),
			CommandName: "edit",
		}
	}

	relPath := RelPath(e.root, resolved)
	return core.Result{
		Output:      fmt.Sprintf("replacement applied in %s", relPath),
		Signal:      core.EditDone,
		CommandName: "edit",
	}
}

// EditBuilder constructs edit commands.
type EditBuilder struct {
	Root string
}

func (b *EditBuilder) Build(res core.Result) core.Command {
	p := ExtractStringParam(res.Output, "path")
	o := ExtractStringParam(res.Output, "old_string")
	n := ExtractStringParam(res.Output, "new_string")
	if p == "" {
		return &FailedParamCmd{ToolName: "edit", Missing: "path"}
	}
	if o == "" {
		return &FailedParamCmd{ToolName: "edit", Missing: "old_string"}
	}
	if n == "" {
		return &FailedParamCmd{ToolName: "edit", Missing: "new_string"}
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

// --- list_files tool ---

type listFilesCmd struct {
	root     string
	path     string
	maxDepth int
}

func (l *listFilesCmd) Name() string { return "list_files" }

func (l *listFilesCmd) Execute() core.Result {
	walkRoot := l.root
	if l.path != "" {
		resolved, err := ValidatePath(l.root, l.path)
		if err != nil {
			return core.Result{
				Output:      fmt.Sprintf("path rejected: %s", err),
				Signal:      core.ToolFailed,
				CommandName: "list_files",
			}
		}
		walkRoot = resolved
	}

	info, err := os.Stat(walkRoot)
	if err != nil {
		return core.Result{
			Output:      fmt.Sprintf("path not found: %s", l.path),
			Signal:      core.ToolFailed,
			CommandName: "list_files",
		}
	}
	if !info.IsDir() {
		return core.Result{
			Output:      fmt.Sprintf("not a directory: %s", l.path),
			Signal:      core.ToolFailed,
			CommandName: "list_files",
		}
	}

	maxDepth := l.maxDepth
	if maxDepth <= 0 {
		maxDepth = 4
	}

	var sb strings.Builder
	walkTree(&sb, walkRoot, l.root, 0, maxDepth)
	return core.Result{
		Output:      sb.String(),
		Signal:      core.ToolDone,
		CommandName: "list_files",
	}
}

func walkTree(sb *strings.Builder, dir, wsRoot string, depth, maxDepth int) {
	if depth >= maxDepth {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		indent := strings.Repeat("  ", depth)
		rel, _ := filepath.Rel(wsRoot, filepath.Join(dir, e.Name()))
		if e.IsDir() {
			fmt.Fprintf(sb, "%s%s/\n", indent, rel)
			walkTree(sb, filepath.Join(dir, e.Name()), wsRoot, depth+1, maxDepth)
		} else {
			fmt.Fprintf(sb, "%s%s\n", indent, rel)
		}
	}
}

// ListFilesBuilder constructs list_files commands.
type ListFilesBuilder struct {
	Root string
}

func (b *ListFilesBuilder) Build(res core.Result) core.Command {
	return &listFilesCmd{
		root:     b.Root,
		path:     ExtractStringParam(res.Output, "path"),
		maxDepth: ExtractIntParam(res.Output, "max_depth"),
	}
}

// ListFilesToolSpec returns the ToolSpec for the list_files tool.
func ListFilesToolSpec() core.ToolSpec {
	return core.ToolSpec{
		Name:        "list_files",
		Description: "List files and directories in a tree format. Use this first to understand the workspace layout before reading files.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Subdirectory to list (default: workspace root)"},"max_depth":{"type":"integer","description":"Maximum directory depth to traverse (default: 4)"}}}`),
		Visibility:  core.External,
	}
}
