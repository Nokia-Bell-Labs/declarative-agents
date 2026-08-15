// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package filesystem

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/monitor"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

type writeCmd struct {
	root         string
	path         string
	content      string
	undoStrategy string
	snapshot     fileSnapshot
	hasSnapshot  bool
	recorder     monitor.ToolMetricsRecorder
	metrics      core.MetricConfig
}

func (w *writeCmd) Name() string { return "write" }
func (w *writeCmd) Undo(prior core.Result) core.Result {
	return undoFileByStrategy(w.Name(), w.undoStrategy, w.root, prior.Receipt, w.snapshot, w.hasSnapshot)
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
	w.recordFilesystemMetric("bytes_written", float64(len(w.content)))
	return core.Result{
		Output:      fmt.Sprintf("wrote %d bytes to %s", len(w.content), RelPath(w.root, resolved)),
		Signal:      core.ToolDone,
		CommandName: "write",
		Receipt:     encodeFileReceipt(snapshot),
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

// WriteBuilder constructs write commands. RootFunc, when set, resolves the
// workspace root at Build time and takes precedence over the static Root —
// nested machines (for example the evaluator point machine) only know their
// workspace directory once an earlier word has created it.
type WriteBuilder struct {
	Root         string
	RootFunc     func() string
	UndoStrategy string
	Metrics      core.MetricConfig
}

func (b *WriteBuilder) root() string {
	if b.RootFunc != nil {
		return b.RootFunc()
	}
	return b.Root
}

func (b *WriteBuilder) Build(res core.Result) core.Command {
	p := extractStringParam(res.Output, "path")
	c, contentPresent := extractStringParamValue(res.Output, "content")
	if p == "" {
		return missingParam("write", "path")
	}
	if !contentPresent {
		return missingParam("write", "content")
	}
	return &writeCmd{root: b.root(), path: p, content: c, undoStrategy: b.UndoStrategy, metrics: b.Metrics}
}

// BuildReverser returns a write command configured only for receipt-driven Undo:
// the receipt carries the prior file state, so the rollback receipt walk needs
// no path/content input (core.Reverser; srd035-checkpoint-port R3).
func (b *WriteBuilder) BuildReverser() core.Command {
	return &writeCmd{root: b.root(), undoStrategy: b.UndoStrategy, metrics: b.Metrics}
}

func undoFileByStrategy(commandName, strategy, root, receipt string, snap fileSnapshot, ok bool) core.Result {
	switch strategy {
	case "", "file_snapshot_restore":
		return undoFileFromReceipt(commandName, root, receipt, snap, ok)
	case "noop":
		return core.NoopUndo(commandName)
	default:
		err := fmt.Errorf("undo %s: unsupported undo strategy %q", commandName, strategy)
		return core.Result{Signal: core.CommandError, CommandName: commandName, Output: err.Error(), Err: err}
	}
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
