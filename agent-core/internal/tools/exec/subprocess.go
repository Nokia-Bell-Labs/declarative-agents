// Copyright (c) 2026 Nokia. All rights reserved.

package exec

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/subprocess"
)

// DefaultOutputLineCap is the default maximum number of output lines before truncation.
const DefaultOutputLineCap = 200

// SubprocessResult maps a shared-transport result to a core.Result. Exec tools
// run with combined output, so the merged stream is on Stdout; a transport
// error is a CommandError, a nonzero exit is a tool-level failure (GH-1393).
func SubprocessResult(name string, r *subprocess.Result) core.Result {
	out := strings.TrimRight(r.Stdout, "\n")
	switch {
	case r.Err != nil:
		return core.Result{
			Output:      out,
			Signal:      core.CommandError,
			Err:         fmt.Errorf("%s: %w", name, r.Err),
			CommandName: name,
		}
	case r.ExitCode != 0:
		return core.Result{Output: out, Signal: core.ToolFailed, CommandName: name}
	default:
		return core.Result{Output: out, Signal: core.ToolDone, CommandName: name}
	}
}

// CapOutput truncates output to maxLines, appending an omission message.
func CapOutput(output string, maxLines int) string {
	lines := strings.Split(output, "\n")
	if len(lines) <= maxLines {
		return output
	}
	kept := strings.Join(lines[:maxLines], "\n")
	omitted := len(lines) - maxLines
	return kept + fmt.Sprintf("\n\n... %d lines omitted", omitted)
}

// ExtractStringParam extracts a string parameter from a JSON tool request.
func ExtractStringParam(jsonOutput, key string) string {
	var params struct {
		Parameters map[string]interface{} `json:"parameters"`
	}
	if err := json.Unmarshal([]byte(jsonOutput), &params); err != nil {
		return ""
	}
	if v, ok := params.Parameters[key].(string); ok {
		return v
	}
	return ""
}

// ExtractIntParam extracts an integer parameter from a JSON tool request.
func ExtractIntParam(jsonOutput, key string) int {
	var params struct {
		Parameters map[string]interface{} `json:"parameters"`
	}
	if err := json.Unmarshal([]byte(jsonOutput), &params); err != nil {
		return 0
	}
	if v, ok := params.Parameters[key].(float64); ok {
		return int(v)
	}
	return 0
}

// FailedParamCmd is returned by builders when required parameters are missing.
type FailedParamCmd struct {
	ToolName string
	Missing  string
}

func (f *FailedParamCmd) Name() string                   { return f.ToolName }
func (f *FailedParamCmd) Undo(_ core.Result) core.Result { return core.NoopUndo(f.Name()) }

func (f *FailedParamCmd) Execute() core.Result {
	return core.Result{
		Output:      fmt.Sprintf("missing required parameter: %s", f.Missing),
		Signal:      core.ToolFailed,
		CommandName: f.ToolName,
	}
}
