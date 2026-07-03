// Copyright (c) 2026 Nokia. All rights reserved.

package llm

import (
	"fmt"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// Undo restores the conversation to its pre-invoke state. It prefers the
// tool-owned receipt on the prior Result (so a fresh command instance can undo
// after a process restart) and falls back to truncating the shared history on
// the live in-process path (srd035-checkpoint-port R3; #44 R2, R3).
func (c *invokeLLMCmd) Undo(prior core.Result) core.Result {
	if msgs, ok, err := decodeConversationReceipt(prior.Receipt); err != nil {
		e := fmt.Errorf("undo invoke_llm: decode receipt: %w", err)
		return core.Result{Signal: core.CommandError, CommandName: c.Name(), Output: e.Error(), Err: e}
	} else if ok {
		c.history.Restore(msgs)
		return core.Result{
			Signal: core.ToolDone, CommandName: c.Name(),
			Output: fmt.Sprintf("undo: restored conversation to %d messages", len(msgs)),
		}
	}
	if !c.hasSnapshot {
		err := fmt.Errorf("undo invoke_llm: no conversation snapshot recorded")
		return core.Result{Signal: core.CommandError, CommandName: c.Name(), Output: err.Error(), Err: err}
	}
	if err := c.history.TruncateTo(c.prevLen); err != nil {
		return core.Result{Signal: core.CommandError, CommandName: c.Name(), Output: err.Error(), Err: err}
	}
	return core.Result{
		Signal: core.ToolDone, CommandName: c.Name(),
		Output: fmt.Sprintf("undo: restored conversation to %d messages", c.prevLen),
	}
}
