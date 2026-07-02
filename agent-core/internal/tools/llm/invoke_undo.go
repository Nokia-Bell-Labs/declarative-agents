// Copyright (c) 2026 Nokia. All rights reserved.

package llm

import (
	"fmt"

	modelllm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func (c *invokeLLMCmd) Undo() core.Result {
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

func (c *invokeLLMCmd) UndoMemento() (core.UndoMemento, error) {
	if !c.hasSnapshot {
		return core.UndoMemento{}, fmt.Errorf("%w: no conversation snapshot recorded for %s", core.ErrUndoMementoMissing, c.Name())
	}
	payload := struct {
		Conversation []modelllm.Message `json:"conversation"`
	}{Conversation: c.prevMessages}
	return core.NewUndoMemento(c.Name(), core.UndoMementoReversible, payload)
}
