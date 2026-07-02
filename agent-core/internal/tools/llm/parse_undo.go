// Copyright (c) 2026 Nokia. All rights reserved.

package llm

import (
	"fmt"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func (p *parseResponseCmd) Undo() core.Result {
	if p.retry == nil {
		return core.NoopUndo(p.Name())
	}
	if !p.hasSnapshot {
		err := fmt.Errorf("undo parse_response: no retry counter snapshot recorded")
		return core.Result{Signal: core.CommandError, CommandName: p.Name(), Output: err.Error(), Err: err}
	}
	p.retry.Restore(p.prevRetries)
	return core.Result{
		Signal: core.ToolDone, CommandName: p.Name(),
		Output: fmt.Sprintf("undo: restored parse retry counter to %d", p.prevRetries),
	}
}

func (p *parseResponseCmd) UndoMemento() (core.UndoMemento, error) {
	if p.retry == nil {
		return core.NoopUndoMemento(p.Name()), nil
	}
	if !p.hasSnapshot {
		return core.UndoMemento{}, fmt.Errorf("%w: no retry counter snapshot recorded for %s", core.ErrUndoMementoMissing, p.Name())
	}
	return retryCounterMemento(p.Name(), p.prevRetries)
}

func retryCounterMemento(commandName string, retries int) (core.UndoMemento, error) {
	payload := struct {
		DomainState struct {
			ParseRetryCounter int `json:"parse_retry_counter"`
		} `json:"domain_state"`
	}{}
	payload.DomainState.ParseRetryCounter = retries
	return core.NewUndoMemento(commandName, core.UndoMementoReversible, payload)
}
