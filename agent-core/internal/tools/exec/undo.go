// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package exec

import (
	"encoding/json"
	"fmt"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

// execReceipt is the opaque, tool-owned rollback context an exec tool encodes
// into Result.Receipt during Execute. It carries the declared reversibility
// tier and the compensation inputs a fresh command instance (or the lifecycle
// receipt walk) needs to reverse the effect after a process restart
// (srd035-checkpoint-port R3; #44 R2). Only the exec tool decodes it.
type execReceipt struct {
	Strategy       string         `json:"strategy"`
	Description    string         `json:"description,omitempty"`
	WorkspacePaths []string       `json:"workspace_paths,omitempty"`
	Requires       []string       `json:"requires,omitempty"`
	Captures       map[string]any `json:"captures,omitempty"`
}

// encodeReceipt serializes the declared undo contract into an opaque receipt.
// Read-only / no-op tools carry no receipt (#44 R2).
func (c *ExecCmd) encodeReceipt(output string) string {
	if c.def.Reversibility.Classification == "irreversible" {
		return ""
	}
	strategy := c.def.Undo.Strategy
	if strategy == "" || strategy == "noop" {
		return ""
	}
	b, err := json.Marshal(execReceipt{
		Strategy:       strategy,
		Description:    c.def.Undo.Description,
		WorkspacePaths: workspacePaths(c.def),
		Requires:       append([]string(nil), c.def.Undo.Requires...),
		Captures:       c.captureUndoValues(output),
	})
	if err != nil {
		return ""
	}
	return string(b)
}

// captureUndoValues projects only declaration-named values into the opaque exec
// receipt. Parameters win over same-named output fields; no tracker-specific or
// other domain field is hard-coded into the generic exec transport.
func (c *ExecCmd) captureUndoValues(output string) map[string]any {
	if len(c.def.Undo.Captures) == 0 {
		return nil
	}
	var decoded map[string]any
	_ = json.Unmarshal([]byte(output), &decoded)
	captures := make(map[string]any)
	for _, name := range c.def.Undo.Captures {
		if value, ok := c.params[name]; ok {
			captures[name] = value
			continue
		}
		if value, ok := decoded[name]; ok {
			captures[name] = value
		}
	}
	if len(captures) == 0 {
		return nil
	}
	return captures
}

func decodeExecReceipt(receipt string) (execReceipt, bool, error) {
	if receipt == "" {
		return execReceipt{}, false, nil
	}
	var r execReceipt
	if err := json.Unmarshal([]byte(receipt), &r); err != nil {
		return execReceipt{}, false, err
	}
	return r, true, nil
}

// workspacePaths collects the declared filesystem-write paths from a tool's side
// effects, defaulting to the workspace root when none are declared.
func workspacePaths(def catalog.ToolDef) []string {
	var paths []string
	for _, effect := range def.SideEffects.Items {
		paths = append(paths, effect.Paths...)
	}
	if len(paths) == 0 {
		paths = []string{"."}
	}
	return paths
}

func compensationUndo(commandName, description string) core.Result {
	return core.Result{
		Signal:      core.CompensationRequired,
		CommandName: commandName,
		Output:      fmt.Sprintf("undo %s requires compensating action: %s", commandName, description),
	}
}
