// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package exec

import (
	"encoding/json"
	"fmt"
	"strconv"
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

// ExtractListParam extracts an array parameter from a JSON tool request as
// its elements' text: strings as they are, numbers in decimal. A lone
// string is a list of one (srd023 R2.12).
func ExtractListParam(jsonOutput, key string) []string {
	var params struct {
		Parameters map[string]interface{} `json:"parameters"`
	}
	if err := json.Unmarshal([]byte(jsonOutput), &params); err != nil {
		return nil
	}
	var out []string
	add := func(v interface{}) {
		switch e := v.(type) {
		case string:
			if e != "" {
				out = append(out, e)
			}
		case float64:
			out = append(out, strconv.FormatFloat(e, 'f', -1, 64))
		}
	}
	switch v := params.Parameters[key].(type) {
	case []interface{}:
		for _, e := range v {
			add(e)
		}
	default:
		add(v)
	}
	return out
}

// ExtractBoolParam reports whether a JSON tool request sets a parameter
// true, as a JSON boolean or the string "true" (srd023 R2.4).
func ExtractBoolParam(jsonOutput, key string) bool {
	var params struct {
		Parameters map[string]interface{} `json:"parameters"`
	}
	if err := json.Unmarshal([]byte(jsonOutput), &params); err != nil {
		return false
	}
	switch v := params.Parameters[key].(type) {
	case bool:
		return v
	case string:
		return v == "true"
	}
	return false
}

// hasParam reports whether a JSON tool request names the parameter at all.
func hasParam(jsonOutput, key string) bool {
	var params struct {
		Parameters map[string]interface{} `json:"parameters"`
	}
	if err := json.Unmarshal([]byte(jsonOutput), &params); err != nil {
		return false
	}
	_, ok := params.Parameters[key]
	return ok
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

// paramType is the JSON type of a parameter a tool request names: string,
// number, boolean, array, or object; empty when the request leaves it out
// or gives null.
func paramType(jsonOutput, key string) string {
	var params struct {
		Parameters map[string]interface{} `json:"parameters"`
	}
	if err := json.Unmarshal([]byte(jsonOutput), &params); err != nil {
		return ""
	}
	switch params.Parameters[key].(type) {
	case string:
		return "string"
	case float64:
		return "number"
	case bool:
		return "boolean"
	case []interface{}:
		return "array"
	case map[string]interface{}:
		return "object"
	}
	return ""
}

// FailedParamCmd is returned by builders when a required parameter is
// missing, or when one is present with a JSON type the tool cannot take:
// Got names that type and Want the one the tool declares, so the model
// fixes the call instead of resending it.
type FailedParamCmd struct {
	ToolName string
	Missing  string
	Got      string
	Want     string
}

func (f *FailedParamCmd) Name() string                   { return f.ToolName }
func (f *FailedParamCmd) Undo(_ core.Result) core.Result { return core.NoopUndo(f.Name()) }

func (f *FailedParamCmd) message() string {
	if f.Got != "" {
		return fmt.Sprintf("parameter %s must be %s, got %s", f.Missing, f.Want, f.Got)
	}
	return fmt.Sprintf("missing required parameter: %s", f.Missing)
}

func (f *FailedParamCmd) Execute() core.Result {
	return core.Result{
		Output:      f.message(),
		Signal:      core.ToolFailed,
		CommandName: f.ToolName,
	}
}
