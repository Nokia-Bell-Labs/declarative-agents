// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
)

// --- find tool ---

type findCmd struct {
	root          string
	query         string
	searchPath    string
	outputLineCap int
}

func (f *findCmd) Name() string { return "find" }

func (f *findCmd) Execute() core.Result {
	args := []string{"--no-heading", "--line-number", f.query}

	if f.searchPath != "" {
		resolved, err := ValidatePath(f.root, f.searchPath)
		if err != nil {
			return core.Result{
				Output:      fmt.Sprintf("path rejected: %s", err),
				Signal:      core.ToolFailed,
				CommandName: "find",
			}
		}
		args = append(args, resolved)
	}

	cmd := exec.Command("rg", args...)
	if f.searchPath == "" {
		cmd.Dir = f.root
	}
	out, err := cmd.CombinedOutput()
	output := strings.TrimRight(string(out), "\n")

	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			if exitErr.ExitCode() == 1 {
				return core.Result{
					Output:      "",
					Signal:      core.ToolDone,
					CommandName: "find",
				}
			}
			return core.Result{
				Signal:      core.ToolFailed,
				Output:      fmt.Sprintf("find failed (exit %d): %s\nNote: query uses ripgrep regex syntax, not glob. Use \\.go$ to match Go files.", exitErr.ExitCode(), output),
				CommandName: "find",
			}
		}
		return core.Result{
			Signal:      core.ToolFailed,
			Output:      fmt.Sprintf("find error: %s", err),
			CommandName: "find",
		}
	}

	if f.outputLineCap > 0 {
		output = CapOutput(output, f.outputLineCap)
	}

	return core.Result{
		Output:      output,
		Signal:      core.ToolDone,
		CommandName: "find",
	}
}

// FindBuilder constructs find commands.
type FindBuilder struct {
	Root          string
	OutputLineCap int
}

func (b *FindBuilder) Build(res core.Result) core.Command {
	q := ExtractStringParam(res.Output, "query")
	if q == "" {
		return &FailedParamCmd{ToolName: "find", Missing: "query"}
	}
	cap := b.OutputLineCap
	if cap == 0 {
		cap = DefaultOutputLineCap
	}
	return &findCmd{
		root:          b.Root,
		query:         q,
		searchPath:    ExtractStringParam(res.Output, "path"),
		outputLineCap: cap,
	}
}

// FindToolSpec returns the ToolSpec for the find tool.
func FindToolSpec() core.ToolSpec {
	return core.ToolSpec{
		Name:        "find",
		Description: "Search for text patterns in the workspace using ripgrep. The query is a regex, not a glob — use \\.go$ to find Go files, not *.go. Returns matching lines with file paths and line numbers.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","description":"Regex pattern (ripgrep syntax, not glob). Example: \\.go$ for Go files, func main for function search"},"path":{"type":"string","description":"Subdirectory to scope the search (optional)"}},"required":["query"]}`),
		Visibility:  core.External,
	}
}
