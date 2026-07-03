// Copyright (c) 2026 Nokia. All rights reserved.

package llm

import (
	"fmt"

	"go.opentelemetry.io/otel/attribute"

	modelllm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

type resetHistoryCmd struct {
	history      *modelllm.Conversation
	tracer       tracing.Tracer
	prevMessages []modelllm.Message
	hasSnapshot  bool
}

func (r *resetHistoryCmd) Name() string { return "reset_history" }

func (r *resetHistoryCmd) Execute() core.Result {
	r.prevMessages = r.history.Snapshot()
	r.hasSnapshot = true
	prevLen := len(r.prevMessages)
	r.history.Reset()
	r.tracer.SetAttributes(attribute.Int("history.cleared_messages", prevLen))
	return core.Result{
		Signal: core.ToolDone, Output: "Begin.", CommandName: r.Name(),
		Receipt: encodeConversationReceipt(r.prevMessages),
	}
}

// Undo restores the cleared conversation, preferring the tool-owned receipt on
// the prior Result and falling back to the in-memory snapshot on the live path
// (srd035-checkpoint-port R3; #44 R2, R3).
func (r *resetHistoryCmd) Undo(prior core.Result) core.Result {
	if msgs, ok, err := decodeConversationReceipt(prior.Receipt); err != nil {
		e := fmt.Errorf("undo reset_history: decode receipt: %w", err)
		return core.Result{Signal: core.CommandError, CommandName: r.Name(), Output: e.Error(), Err: e}
	} else if ok {
		r.history.Restore(msgs)
		return core.Result{Signal: core.ToolDone, CommandName: r.Name(), Output: fmt.Sprintf("undo: restored %d conversation messages", len(msgs))}
	}
	if !r.hasSnapshot {
		err := fmt.Errorf("undo reset_history: no conversation snapshot recorded")
		return core.Result{Signal: core.CommandError, CommandName: r.Name(), Output: err.Error(), Err: err}
	}
	r.history.Restore(r.prevMessages)
	return core.Result{Signal: core.ToolDone, CommandName: r.Name(), Output: fmt.Sprintf("undo: restored %d conversation messages", len(r.prevMessages))}
}

// ResetHistoryBuilder constructs reset_history commands.
type ResetHistoryBuilder struct {
	History *modelllm.Conversation
	Tracer  tracing.Tracer
}

func (b *ResetHistoryBuilder) Build(_ core.Result) core.Command {
	return &resetHistoryCmd{history: b.History, tracer: b.Tracer}
}
