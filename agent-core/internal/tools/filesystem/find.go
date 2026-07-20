// Copyright (c) 2026 Nokia. All rights reserved.

package filesystem

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/subprocess"
)

type findCmd struct {
	root          string
	query         string
	searchPath    string
	outputLineCap int
}

func (f *findCmd) Name() string                   { return "find" }
func (f *findCmd) Undo(_ core.Result) core.Result { return core.NoopUndo(f.Name()) }

func (f *findCmd) Execute() core.Result {
	args, cmdDir, err := f.ripgrepArgs()
	if err != nil {
		return toolFailed("find", fmt.Sprintf("path rejected: %s", err))
	}
	// The ripgrep subprocess runs through the shared, process-group-managed
	// subprocess support rather than the tool spawning it itself, so the tool holds
	// only its search contract and the transport stays in one place (GH-447).
	res := subprocess.Run(context.Background(), subprocess.Spec{Binary: "rg", Args: args, Dir: cmdDir})
	output := strings.TrimRight(res.Stdout, "\n")
	if !res.Success() {
		return findError(res, output)
	}
	if f.outputLineCap > 0 {
		output = capOutput(output, f.outputLineCap)
	}
	return core.Result{Output: output, Signal: core.ToolDone, CommandName: "find"}
}

func (f *findCmd) ripgrepArgs() ([]string, string, error) {
	args := []string{"--no-heading", "--line-number", f.query}
	if f.searchPath == "" {
		return args, f.root, nil
	}
	resolved, err := ValidatePath(f.root, f.searchPath)
	if err != nil {
		return nil, "", err
	}
	return append(args, resolved), "", nil
}

// findError maps a failed ripgrep run to a machine result. ripgrep exits 1 when it
// finds no matches -- a successful empty search, not an error -- and >1 on a real
// failure; a timeout or a spawn failure (for example ripgrep not installed) is an
// infrastructure error.
func findError(res *subprocess.Result, output string) core.Result {
	switch {
	case res.ExitCode == 1:
		return core.Result{Output: "", Signal: core.ToolDone, CommandName: "find"}
	case res.ExitCode > 1:
		msg := fmt.Sprintf("find failed (exit %d): %s\nNote: query uses ripgrep regex syntax, not glob. Use \\.go$ to match Go files.", res.ExitCode, output)
		return toolFailed("find", msg)
	case res.TimedOut:
		return toolFailed("find", "find timed out")
	default:
		return toolFailed("find", fmt.Sprintf("find error: %v", res.Err))
	}
}

// FindBuilder constructs find commands.
type FindBuilder struct {
	Root          string
	OutputLineCap int
}

func (b *FindBuilder) Build(res core.Result) core.Command {
	q := extractStringParam(res.Output, "query")
	if q == "" {
		return missingParam("find", "query")
	}
	cap := b.OutputLineCap
	if cap == 0 {
		cap = defaultOutputLineCap
	}
	return &findCmd{
		root:          b.Root,
		query:         q,
		searchPath:    extractStringParam(res.Output, "path"),
		outputLineCap: cap,
	}
}

// FindToolSpec returns the ToolSpec for the find tool.
func FindToolSpec() core.ToolSpec {
	return core.ToolSpec{
		Name:        "find",
		Description: "Search for text patterns in the workspace using ripgrep. The query is a regex, not a glob - use \\.go$ to find Go files, not *.go. Returns matching lines with file paths and line numbers.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","description":"Regex pattern (ripgrep syntax, not glob). Example: \\.go$ for Go files, func main for function search"},"path":{"type":"string","description":"Subdirectory to scope the search (optional)"}},"required":["query"]}`),
		Visibility:  core.External,
	}
}
