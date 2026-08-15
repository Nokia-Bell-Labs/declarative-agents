// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package filesystem

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

const defaultOutputLineCap = 200

func extractStringParam(jsonOutput, key string) string {
	value, _ := extractStringParamValue(jsonOutput, key)
	return value
}

// extractStringParamValue distinguishes a missing/non-string parameter from a
// present empty string. Empty content is valid for write even though most
// filesystem parameters use an empty value as "not supplied".
func extractStringParamValue(jsonOutput, key string) (string, bool) {
	var params struct {
		Parameters map[string]interface{} `json:"parameters"`
	}
	if err := json.Unmarshal([]byte(jsonOutput), &params); err != nil {
		return "", false
	}
	if v, ok := params.Parameters[key].(string); ok {
		return v, true
	}
	return "", false
}

func extractIntParam(jsonOutput, key string) int {
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

func extractBoolParam(jsonOutput, key string) bool {
	var params struct {
		Parameters map[string]interface{} `json:"parameters"`
	}
	if err := json.Unmarshal([]byte(jsonOutput), &params); err != nil {
		return false
	}
	if v, ok := params.Parameters[key].(bool); ok {
		return v
	}
	return false
}

func missingParam(toolName, missing string) core.Command {
	return failedParamCmd{toolName: toolName, missing: missing}
}

type failedParamCmd struct {
	toolName string
	missing  string
}

func (c failedParamCmd) Name() string                   { return c.toolName }
func (c failedParamCmd) Undo(_ core.Result) core.Result { return core.NoopUndo(c.Name()) }

func (c failedParamCmd) Execute() core.Result {
	return core.Result{
		Signal:      core.ToolFailed,
		Output:      "missing required parameter: " + c.missing,
		CommandName: c.toolName,
	}
}

func capOutput(output string, maxLines int) string {
	lines := strings.Split(output, "\n")
	if len(lines) <= maxLines {
		return output
	}
	kept := strings.Join(lines[:maxLines], "\n")
	omitted := len(lines) - maxLines
	return kept + fmt.Sprintf("\n\n... %d lines omitted", omitted)
}
