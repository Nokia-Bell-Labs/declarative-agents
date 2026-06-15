// Copyright (c) 2026 Nokia. All rights reserved.

package filesystem

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/observability/monitor"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
)

type writeCmd struct {
	root        string
	path        string
	content     string
	snapshot    fileSnapshot
	hasSnapshot bool
	recorder    monitor.ToolMetricsRecorder
}

func (w *writeCmd) Name() string { return "write" }
func (w *writeCmd) Undo() core.Result {
	return undoFileSnapshot(w.Name(), w.root, w.snapshot, w.hasSnapshot)
}
func (w *writeCmd) UndoMemento() (core.UndoMemento, error) {
	return fileWorkspaceMemento(w.Name(), w.snapshot, w.hasSnapshot)
}

func (w *writeCmd) Execute() core.Result {
	resolved, err := writablePath(w.root, w.path)
	if err != nil {
		return toolFailed("write", fmt.Sprintf("path rejected: %s", err))
	}
	snapshot, err := snapshotFile(w.root, resolved)
	if err != nil {
		return commandError("write", fmt.Errorf("write snapshot %s: %w", w.path, err))
	}
	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		return commandError("write", fmt.Errorf("write mkdir %s: %w", w.path, err))
	}
	if err := os.WriteFile(resolved, []byte(w.content), 0o644); err != nil {
		return commandError("write", fmt.Errorf("write %s: %w", w.path, err))
	}
	w.snapshot = snapshot
	w.hasSnapshot = true
	w.recordFilesystemMetric("filesystem.bytes_written", float64(len(w.content)), "By", "Bytes written to one workspace file.")
	return core.Result{
		Output:      fmt.Sprintf("wrote %d bytes to %s", len(w.content), RelPath(w.root, resolved)),
		Signal:      core.ToolDone,
		CommandName: "write",
	}
}

func writablePath(root, path string) (string, error) {
	resolved, err := ValidatePath(root, path)
	if err == nil {
		return resolved, nil
	}
	if !strings.Contains(err.Error(), "not found") {
		return "", err
	}
	joined := path
	if !filepath.IsAbs(path) {
		joined = filepath.Join(root, path)
	}
	return filepath.Clean(joined), nil
}

// WriteBuilder constructs write commands.
type WriteBuilder struct {
	Root string
}

func (b *WriteBuilder) Build(res core.Result) core.Command {
	p := extractStringParam(res.Output, "path")
	c := extractStringParam(res.Output, "content")
	if p == "" {
		return missingParam("write", "path")
	}
	if c == "" {
		return missingParam("write", "content")
	}
	return &writeCmd{root: b.Root, path: p, content: c}
}

// WriteToolSpec returns the ToolSpec for the write tool.
func WriteToolSpec() core.ToolSpec {
	return core.ToolSpec{
		Name:        "write",
		Description: "Create or overwrite a file. Provide the complete file content - this replaces the entire file. Parent directories are created automatically.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"File path relative to workspace root"},"content":{"type":"string","description":"Full file content to write"}},"required":["path","content"]}`),
		Visibility:  core.External,
	}
}

func commandError(name string, err error) core.Result {
	return core.Result{Signal: core.CommandError, Err: err, CommandName: name}
}
