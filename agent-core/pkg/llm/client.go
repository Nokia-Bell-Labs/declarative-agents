// Copyright (c) 2026 Nokia. All rights reserved.

package llm

import "context"

// Client is the stateless LLM backend port. Implementations handle
// the wire protocol for a specific provider (Ollama, OpenAI, vLLM).
//
// Client has no conversation knowledge — it sends messages and returns
// a response. Conversation state is managed by the Conversation type.
type Client interface {
	// Chat sends a sequence of messages to the LLM and returns the
	// assistant's response. The caller is responsible for assembling
	// the full message history including the system prompt.
	Chat(ctx context.Context, messages []Message, opts ChatOptions) (ChatResponse, error)
}
