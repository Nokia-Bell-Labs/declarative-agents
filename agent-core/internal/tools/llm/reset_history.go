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
	return core.Result{Signal: core.ToolDone, Output: "Begin.", CommandName: r.Name()}
}

func (r *resetHistoryCmd) Undo() core.Result {
	if !r.hasSnapshot {
		err := fmt.Errorf("undo reset_history: no conversation snapshot recorded")
		return core.Result{Signal: core.CommandError, CommandName: r.Name(), Output: err.Error(), Err: err}
	}
	r.history.Restore(r.prevMessages)
	return core.Result{Signal: core.ToolDone, CommandName: r.Name(), Output: fmt.Sprintf("undo: restored %d conversation messages", len(r.prevMessages))}
}

func (r *resetHistoryCmd) UndoMemento() (core.UndoMemento, error) {
	if !r.hasSnapshot {
		return core.UndoMemento{}, fmt.Errorf("%w: no conversation snapshot recorded for %s", core.ErrUndoMementoMissing, r.Name())
	}
	payload := struct {
		Conversation []modelllm.Message `json:"conversation"`
	}{Conversation: r.prevMessages}
	return core.NewUndoMemento(r.Name(), core.UndoMementoReversible, payload)
}

// ResetHistoryBuilder constructs reset_history commands.
type ResetHistoryBuilder struct {
	History *modelllm.Conversation
	Tracer  tracing.Tracer
}

func (b *ResetHistoryBuilder) Build(_ core.Result) core.Command {
	return &resetHistoryCmd{history: b.History, tracer: b.Tracer}
}
