// Copyright (c) 2026 Nokia. All rights reserved.

package llm

import (
	"context"
	modelllm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/prompt"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

type fakeChatClient struct {
	response modelllm.ChatResponse
	err      error
}

func (s *fakeChatClient) Chat(_ context.Context, _ []modelllm.Message, _ modelllm.ChatOptions) (modelllm.ChatResponse, error) {
	return s.response, s.err
}

func (s *fakeChatClient) ListModels(_ context.Context) ([]modelllm.ModelInfo, error) {
	return nil, nil
}

type waitClient struct{}

func (w waitClient) Chat(ctx context.Context, _ []modelllm.Message, _ modelllm.ChatOptions) (modelllm.ChatResponse, error) {
	<-ctx.Done()
	return modelllm.ChatResponse{}, ctx.Err()
}

func (w waitClient) ListModels(_ context.Context) ([]modelllm.ModelInfo, error) {
	return nil, nil
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
