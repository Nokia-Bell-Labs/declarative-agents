// Copyright (c) 2026 Nokia. All rights reserved.

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
	Strategy       string   `json:"strategy"`
	Description    string   `json:"description,omitempty"`
	WorkspacePaths []string `json:"workspace_paths,omitempty"`
	Requires       []string `json:"requires,omitempty"`
	IssueID        string   `json:"issue_id,omitempty"`
}

// encodeReceipt serializes the declared undo contract into an opaque receipt.
// Read-only / no-op tools carry no receipt (#44 R2).
func (c *ExecCmd) encodeReceipt() string {
	strategy := c.def.Undo.Strategy
	if strategy == "" || strategy == "noop" {
		return ""
	}
	b, err := json.Marshal(execReceipt{
		Strategy:       strategy,
		Description:    c.def.Undo.Description,
		WorkspacePaths: workspacePaths(c.def),
		Requires:       append([]string(nil), c.def.Undo.Requires...),
		IssueID:        c.params["id"],
	})
	if err != nil {
		return ""
	}
	return string(b)
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
	err := fmt.Errorf("undo %s requires compensating action: %s", commandName, description)
	return core.Result{Signal: core.CommandError, CommandName: commandName, Output: err.Error(), Err: err}
}
