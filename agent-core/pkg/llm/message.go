// Copyright (c) 2026 Nokia. All rights reserved.

// Package llm defines the generic LLM infrastructure for agents built
// on the agent-core framework. It provides a stateless Client interface
// (the LLM backend port) and a stateful Conversation type that manages
// multi-turn chat sessions with lifecycle and history access.
package llm

import "encoding/json"

// MessageRole identifies the speaker of a conversation message.
type MessageRole string

const (
	System    MessageRole = "system"
	User      MessageRole = "user"
	Assistant MessageRole = "assistant"
)

// Message is a single entry in a conversation.
type Message struct {
	Role    MessageRole `json:"role"`
	Content string      `json:"content"`
}

// ChatOptions configures a single LLM chat call.
type ChatOptions struct {
	Model       string  `json:"model"`
	Temperature float64 `json:"temperature"`
	Seed        int     `json:"seed"`
	NumCtx      int     `json:"num_ctx,omitempty"`
}

// ChatResponse is the result of a single LLM chat call.
type ChatResponse struct {
	Content  string `json:"content"`
	TokensIn int    `json:"tokens_in"`
	TokensOut int   `json:"tokens_out"`
}

// ToolRequest is the intermediate representation of a parsed tool call.
// The LLM produces JSON matching this shape; the agent loop dispatches
// based on ToolName and forwards Params to the resolved builder.
type ToolRequest struct {
	ToolName string          `json:"tool"`
	Params   json.RawMessage `json:"parameters"`
}
