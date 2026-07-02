// Copyright (c) 2026 Nokia. All rights reserved.

package filesystem

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/monitor"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

type readCmd struct {
	root      string
	path      string
	startLine int
	endLine   int
	recorder  monitor.ToolMetricsRecorder
	metrics   core.MetricConfig
}

func (r *readCmd) Name() string      { return "read" }
func (r *readCmd) Undo() core.Result { return core.NoopUndo(r.Name()) }

func (r *readCmd) Execute() core.Result {
	resolved, err := ValidatePath(r.root, r.path)
	if err != nil {
		return toolFailed("read", fmt.Sprintf("path rejected: %s", err))
	}
	info, statErr := os.Stat(resolved)
	if statErr == nil && info.IsDir() {
		return readDirFailure(r.path, resolved)
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		return readFileError(r.path, err)
	}
	if IsBinary(data) {
		return toolFailed("read", fmt.Sprintf("file appears to be binary: %s", RelPath(r.root, resolved)))
	}
	r.recordFilesystemMetric("bytes_read", float64(len(data)))
	return readLines(data, r.startLine, r.endLine)
}

func readDirFailure(path, resolved string) core.Result {
	entries, _ := os.ReadDir(resolved)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	msg := fmt.Sprintf("path is a directory, not a file: %s\nContents: %s\nUse read on a specific file.", path, strings.Join(names, ", "))
	return toolFailed("read", msg)
}

func readFileError(path string, err error) core.Result {
	if os.IsNotExist(err) {
		return toolFailed("read", fmt.Sprintf("file not found: %s", path))
	}
	return toolFailed("read", fmt.Sprintf("cannot read %s: %s", path, err))
}

func readLines(data []byte, startLine, endLine int) core.Result {
	lines := strings.Split(string(data), "\n")
	start, end := normalizedLineRange(len(lines), startLine, endLine)
	if start > len(lines) {
		return core.Result{Output: "", Signal: core.ToolDone, CommandName: "read"}
	}
	var sb strings.Builder
	width := len(fmt.Sprintf("%d", end))
	for i := start; i <= end; i++ {
		fmt.Fprintf(&sb, "%*d|%s\n", width, i, lines[i-1])
	}
	return core.Result{Output: strings.TrimRight(sb.String(), "\n"), Signal: core.ToolDone, CommandName: "read"}
}

func normalizedLineRange(lineCount, startLine, endLine int) (int, int) {
	start, end := startLine, endLine
	if start <= 0 {
		start = 1
	}
	if end <= 0 || end > lineCount {
		end = lineCount
	}
	return start, end
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
	Root    string
	Metrics core.MetricConfig
}

func (b *ReadBuilder) Build(res core.Result) core.Command {
	p := extractStringParam(res.Output, "path")
	if p == "" {
		return missingParam("read", "path")
	}
	return &readCmd{
		root:      b.Root,
		path:      p,
		startLine: extractIntParam(res.Output, "start_line"),
		endLine:   extractIntParam(res.Output, "end_line"),
		metrics:   b.Metrics,
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

func toolFailed(name, output string) core.Result {
	return core.Result{Output: output, Signal: core.ToolFailed, CommandName: name}
}
