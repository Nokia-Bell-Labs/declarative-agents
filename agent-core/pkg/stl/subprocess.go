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

// DefaultOutputLineCap is the default maximum number of output lines
// before truncation.
const DefaultOutputLineCap = 200

// SubprocessResult maps subprocess output and error to a core.Result.
// Exit errors produce ToolFailed; other errors produce CommandError.
func SubprocessResult(name string, output []byte, err error) core.Result {
	out := strings.TrimRight(string(output), "\n")
	if err == nil {
		return core.Result{
			Output:      out,
			Signal:      core.ToolDone,
			CommandName: name,
		}
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return core.Result{
			Output:      out,
			Signal:      core.ToolFailed,
			CommandName: name,
		}
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
// Expected format: {"parameters": {"key": "value"}}
func ExtractStringParam(jsonOutput, key string) string {
	var params struct {
		Parameters map[string]interface{} `json:"parameters"`
	}
	if err := json.Unmarshal([]byte(jsonOutput), &params); err != nil {
		return ""
	}
	v, ok := params.Parameters[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

// ExtractIntParam extracts an integer parameter from a JSON tool request.
func ExtractIntParam(jsonOutput, key string) int {
	var params struct {
		Parameters map[string]interface{} `json:"parameters"`
	}
	if err := json.Unmarshal([]byte(jsonOutput), &params); err != nil {
		return 0
	}
	v, ok := params.Parameters[key]
	if !ok {
		return 0
	}
	f, ok := v.(float64)
	if !ok {
		return 0
	}
	return int(f)
}

// FailedParamCmd is a command returned by builders when required
// parameters are missing from the tool request.
type FailedParamCmd struct {
	ToolName string
	Missing  string
}

func (f *FailedParamCmd) Name() string      { return f.ToolName }
func (f *FailedParamCmd) Undo() core.Result { return core.NoopUndo(f.Name()) }

func (f *FailedParamCmd) Execute() core.Result {
	return core.Result{
		Output:      fmt.Sprintf("missing required parameter: %s", f.Missing),
		Signal:      core.ToolFailed,
		CommandName: f.ToolName,
	}
}
