// Copyright (c) 2026 Nokia. All rights reserved.

package llm

import (
	"fmt"
	"strings"

	"go.opentelemetry.io/otel/attribute"

	modelllm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

type resetHistoryCmd struct {
	toolName     string
	history      *modelllm.Conversation
	tracer       tracing.Tracer
	prevMessages []modelllm.Message
	prevRef      string
	hasSnapshot  bool
	refProvider  ConversationReferenceProvider
	refResolver  ConversationReferenceResolver
}

func (r *resetHistoryCmd) Name() string {
	if r.toolName != "" {
		return r.toolName
	}
	return "reset_history"
}
func (r *resetHistoryCmd) SerialDispatchOnly() {}

var _ core.SerialDispatchOnly = (*resetHistoryCmd)(nil)

func (r *resetHistoryCmd) Execute() core.Result {
	r.prevMessages = r.history.Snapshot()
	r.hasSnapshot = true
	prevLen := len(r.prevMessages)
	r.prevRef = ""
	if r.refProvider != nil {
		if ref, ok := r.refProvider.ConversationReference(); ok {
			r.prevRef = strings.TrimSpace(ref)
		}
	}
	r.history.Reset()
	r.tracer.SetAttributes(attribute.Int("history.cleared_messages", prevLen))
	return core.Result{
		Signal: core.ToolDone, Output: "Begin.", CommandName: r.Name(),
		Receipt: encodeConversationReceipt(prevLen, r.prevRef),
	}
}

// Undo restores the cleared conversation. Legacy receipts carry the full
// snapshot; v2 receipts use the command-local snapshot in process or resolve an
// authoritative checkpoint reference after restart.
func (r *resetHistoryCmd) Undo(prior core.Result) core.Result {
	receipt, ok, err := decodeConversationReceipt(prior.Receipt)
	if err != nil {
		e := fmt.Errorf("undo reset_history: decode receipt: %w", err)
		return core.Result{Signal: core.CommandError, CommandName: r.Name(), Output: e.Error(), Err: e}
	}
	if ok && receipt.legacy {
		r.history.Restore(receipt.legacyConversation)
		return core.Result{Signal: core.ToolDone, CommandName: r.Name(), Output: fmt.Sprintf("undo: restored %d conversation messages", receipt.priorLength)}
	}
	if r.hasSnapshot && (!ok || len(r.prevMessages) == receipt.priorLength) {
		r.history.Restore(r.prevMessages)
		return core.Result{Signal: core.ToolDone, CommandName: r.Name(), Output: fmt.Sprintf("undo: restored %d conversation messages", len(r.prevMessages))}
	}
	if !ok {
		err := fmt.Errorf("undo reset_history: no conversation snapshot recorded")
		return core.Result{Signal: core.CommandError, CommandName: r.Name(), Output: err.Error(), Err: err}
	}

	messages, err := resolveConversationReceipt(receipt, r.refResolver)
	if err != nil {
		e := fmt.Errorf("undo reset_history: %w", err)
		return core.Result{Signal: core.CommandError, CommandName: r.Name(), Output: e.Error(), Err: e}
	}
	r.history.Restore(messages)
	return core.Result{Signal: core.ToolDone, CommandName: r.Name(), Output: fmt.Sprintf("undo: restored %d conversation messages", len(messages))}
}

// ResetHistoryBuilder constructs reset_history commands.
type ResetHistoryBuilder struct {
	ToolName                string
	History                 *modelllm.Conversation
	Tracer                  tracing.Tracer
	ConversationRefProvider ConversationReferenceProvider
	ConversationRefResolver ConversationReferenceResolver
}

func (b *ResetHistoryBuilder) Build(_ core.Result) core.Command {
	return &resetHistoryCmd{
		toolName: b.ToolName,
		history:  b.History, tracer: b.Tracer,
		refProvider: b.ConversationRefProvider,
		refResolver: b.ConversationRefResolver,
	}
}

// BuildReverser constructs the receipt-only reset command used after restart.
func (b *ResetHistoryBuilder) BuildReverser() core.Command {
	return &resetHistoryCmd{
		toolName: b.ToolName,
		history:  b.History, tracer: b.Tracer,
		refResolver: b.ConversationRefResolver,
	}
}

var _ core.Reverser = (*ResetHistoryBuilder)(nil)
