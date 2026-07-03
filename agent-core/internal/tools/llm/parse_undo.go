// Copyright (c) 2026 Nokia. All rights reserved.

package llm

import (
	"fmt"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// Undo restores the parse-retry counter. It prefers the tool-owned receipt on
// the prior Result and falls back to the in-memory snapshot on the live path
// (srd035-checkpoint-port R3; #44 R2, R3).
func (p *parseResponseCmd) Undo(prior core.Result) core.Result {
	if p.retry == nil {
		return core.NoopUndo(p.Name())
	}
	if retries, ok, err := decodeRetryReceipt(prior.Receipt); err != nil {
		e := fmt.Errorf("undo parse_response: decode receipt: %w", err)
		return core.Result{Signal: core.CommandError, CommandName: p.Name(), Output: e.Error(), Err: e}
	} else if ok {
		p.retry.Restore(retries)
		return core.Result{
			Signal: core.ToolDone, CommandName: p.Name(),
			Output: fmt.Sprintf("undo: restored parse retry counter to %d", retries),
		}
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
