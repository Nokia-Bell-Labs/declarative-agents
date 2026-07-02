// Copyright (c) 2026 Nokia. All rights reserved.

package filesystem

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
)

type listFilesCmd struct {
	root     string
	path     string
	maxDepth int
}

func (l *listFilesCmd) Name() string      { return "list_files" }
func (l *listFilesCmd) Undo() core.Result { return core.NoopUndo(l.Name()) }

func (l *listFilesCmd) Execute() core.Result {
	walkRoot, failure := l.walkRoot()
	if failure.Signal != "" {
		return failure
	}
	maxDepth := l.maxDepth
	if maxDepth <= 0 {
		maxDepth = 4
	}
	var sb strings.Builder
	walkTree(&sb, walkRoot, l.root, 0, maxDepth)
	return core.Result{Output: sb.String(), Signal: core.ToolDone, CommandName: "list_files"}
}

func (l *listFilesCmd) walkRoot() (string, core.Result) {
	walkRoot := l.root
	if l.path != "" {
		resolved, err := ValidatePath(l.root, l.path)
		if err != nil {
			return "", toolFailed("list_files", fmt.Sprintf("path rejected: %s", err))
		}
		walkRoot = resolved
	}
	info, err := os.Stat(walkRoot)
	if err != nil {
		return "", toolFailed("list_files", fmt.Sprintf("path not found: %s", l.path))
	}
	if !info.IsDir() {
		return "", toolFailed("list_files", fmt.Sprintf("not a directory: %s", l.path))
	}
	return walkRoot, core.Result{}
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
		writeTreeEntry(sb, dir, wsRoot, e, depth, maxDepth)
	}
}

func writeTreeEntry(sb *strings.Builder, dir, wsRoot string, e os.DirEntry, depth, maxDepth int) {
	indent := strings.Repeat("  ", depth)
	path := filepath.Join(dir, e.Name())
	rel, _ := filepath.Rel(wsRoot, path)
	if e.IsDir() {
		fmt.Fprintf(sb, "%s%s/\n", indent, rel)
		walkTree(sb, path, wsRoot, depth+1, maxDepth)
		return
	}
	fmt.Fprintf(sb, "%s%s\n", indent, rel)
}

// ListFilesBuilder constructs list_files commands.
type ListFilesBuilder struct {
	Root string
}

func (b *ListFilesBuilder) Build(res core.Result) core.Command {
	return &listFilesCmd{
		root:     b.Root,
		path:     extractStringParam(res.Output, "path"),
		maxDepth: extractIntParam(res.Output, "max_depth"),
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
