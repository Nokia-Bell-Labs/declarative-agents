// Copyright (c) 2026 Nokia. All rights reserved.

package exec

import (
	"encoding/json"
	"errors"
	"fmt"
	osexec "os/exec"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// DefaultOutputLineCap is the default maximum number of output lines before truncation.
const DefaultOutputLineCap = 200

// SubprocessResult maps subprocess output and error to a core.Result.
func SubprocessResult(name string, output []byte, err error) core.Result {
	out := strings.TrimRight(string(output), "\n")
	if err == nil {
		return core.Result{Output: out, Signal: core.ToolDone, CommandName: name}
	}
	var exitErr *osexec.ExitError
	if errors.As(err, &exitErr) {
		return core.Result{Output: out, Signal: core.ToolFailed, CommandName: name}
	}
	return core.Result{
		Output:      out,
		Signal:      core.CommandError,
		Err:         fmt.Errorf("%s: %w", name, err),
		CommandName: name,
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
