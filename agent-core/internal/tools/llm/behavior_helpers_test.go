// Copyright (c) 2026 Nokia. All rights reserved.

package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	modelllm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/prompt"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/stretchr/testify/require"
)

type fakeChatClient struct {
	response modelllm.ChatResponse
	err      error
}

func (s *fakeChatClient) Chat(_ context.Context, _ []modelllm.Message, _ modelllm.ChatOptions) (modelllm.ChatResponse, error) {
	return s.response, s.err
}

type waitClient struct{}

func (w waitClient) Chat(ctx context.Context, _ []modelllm.Message, _ modelllm.ChatOptions) (modelllm.ChatResponse, error) {
	<-ctx.Done()
	return modelllm.ChatResponse{}, ctx.Err()
}

type fakeAssembler struct{}

func (s *fakeAssembler) AssembleMessages(conv *modelllm.Conversation, _ *core.Registry, _ core.State) []modelllm.Message {
	msgs := []modelllm.Message{{Role: modelllm.System, Content: "You are a helper."}}
	msgs = append(msgs, conv.Messages()...)
	return msgs
}

type fakeParser struct{}

func (s *fakeParser) ExtractToolCall(raw string) string {
	return modelllm.ExtractBraces(raw)
}

func (s *fakeParser) EnvelopeConfig() (*prompt.Envelope, bool) {
	return nil, false
}

func noopTracer() tracing.Tracer {
	return tracing.NoopTracer{}
}

type fakeConversationReferenceResolver struct {
	conversations map[string][]modelllm.Message
	err           error
}

func (f fakeConversationReferenceResolver) ResolveConversationReference(reference string) ([]modelllm.Message, error) {
	if f.err != nil {
		return nil, f.err
	}
	messages, ok := f.conversations[reference]
	if !ok {
		return nil, fmt.Errorf("conversation reference not found")
	}
	result := make([]modelllm.Message, len(messages))
	copy(result, messages)
	return result, nil
}

func encodeLegacyConversationReceipt(t *testing.T, messages []modelllm.Message) string {
	t.Helper()
	receipt, err := json.Marshal(legacyConversationReceipt{Conversation: messages})
	require.NoError(t, err)
	return string(receipt)
}
