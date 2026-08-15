// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package llm

import (
	"fmt"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// Undo restores the conversation to its pre-invoke state. Legacy receipts carry
// the full snapshot; v2 receipts use command-local state in process or resolve
// an authoritative checkpoint reference after restart.
func (c *invokeLLMCmd) Undo(prior core.Result) core.Result {
	receipt, ok, err := decodeConversationReceipt(prior.Receipt)
	if err != nil {
		return c.undoError(fmt.Errorf("decode receipt: %w", err))
	}
	if ok && receipt.legacy {
		c.history.Restore(receipt.legacyConversation)
		return c.undoSuccess(receipt.priorLength)
	}
	return c.undoV2OrLocal(receipt, ok)
}

func (c *invokeLLMCmd) undoV2OrLocal(receipt decodedConversationReceipt, hasReceipt bool) core.Result {
	targetLength := c.prevLen
	if hasReceipt {
		targetLength = receipt.priorLength
	}
	if c.hasSnapshot && c.prevLen == targetLength {
		if err := c.history.TruncateTo(targetLength); err != nil {
			return c.undoError(err)
		}
		return c.undoSuccess(targetLength)
	}
	if !hasReceipt {
		return c.undoError(fmt.Errorf("no conversation snapshot recorded"))
	}

	messages, err := resolveConversationReceipt(receipt, c.conversationRefResolver)
	if err != nil {
		return c.undoError(err)
	}
	c.history.Restore(messages)
	return c.undoSuccess(len(messages))
}

func (c *invokeLLMCmd) undoSuccess(length int) core.Result {
	return core.Result{
		Signal: core.ToolDone, CommandName: c.Name(),
		Output: fmt.Sprintf("undo: restored conversation to %d messages", length),
	}
}

func (c *invokeLLMCmd) undoError(err error) core.Result {
	err = fmt.Errorf("undo invoke_llm: %w", err)
	return core.Result{Signal: core.CommandError, CommandName: c.Name(), Output: err.Error(), Err: err}
}
