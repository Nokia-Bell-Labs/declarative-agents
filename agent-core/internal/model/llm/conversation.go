// Copyright (c) 2026 Nokia. All rights reserved.

package llm

import (
	"context"
	"fmt"
	"sync"
)

// Conversation manages a multi-turn chat session with an LLM backend.
// It owns the conversation history and provides lifecycle methods for
// sending messages, inspecting history, and resetting state.
//
// A Conversation is created with a Client, a system prompt, and chat
// options. Each Send call appends a user message, invokes the Client
// with the full history, and appends the assistant's response.
type Conversation struct {
	client       Client
	systemPrompt string
	opts         ChatOptions

	mu       sync.RWMutex
	messages []Message
}

// NewConversation creates a conversation bound to the given Client.
// The system prompt is prepended to every Chat call automatically.
func NewConversation(client Client, systemPrompt string, opts ChatOptions) *Conversation {
	return &Conversation{
		client:       client,
		systemPrompt: systemPrompt,
		opts:         opts,
	}
}

// Send appends a user message, calls the LLM with the full conversation
// history (system prompt + all messages), and appends the assistant's
// response. Returns the ChatResponse from the backend.
//
// If the Chat call fails, the user message is still appended but no
// assistant message is added, preserving the ability to retry.
func (c *Conversation) Send(ctx context.Context, userMessage string) (ChatResponse, error) {
	c.mu.Lock()
	c.messages = append(c.messages, Message{Role: User, Content: userMessage})

	chatMessages := c.assembleMessagesLocked()
	c.mu.Unlock()

	resp, err := c.client.Chat(ctx, chatMessages, c.opts)
	if err != nil {
		return ChatResponse{}, err
	}

	c.mu.Lock()
	c.messages = append(c.messages, Message{Role: Assistant, Content: resp.Content})
	c.mu.Unlock()

	return resp, nil
}

// Append adds a message to the conversation history without triggering
// a Chat call. This supports the manual/append-only pattern where the
// caller builds the history independently and invokes Chat externally.
//
// Use Send for the managed pattern (auto-calls Chat). Use Append when
// assembling messages from multiple sources before a single Chat call.
func (c *Conversation) Append(msg Message) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messages = append(c.messages, msg)
}

// History returns a copy of the conversation messages (excluding the
// system prompt). Messages are in insertion order, oldest first.
func (c *Conversation) History() []Message {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]Message, len(c.messages))
	copy(out, c.messages)
	return out
}

// Snapshot returns a copy of the current conversation history.
// It is equivalent to History and named for rollback callers.
func (c *Conversation) Snapshot() []Message {
	return c.History()
}

// Restore replaces the conversation history with the provided messages.
// The system prompt, client, and options are preserved.
func (c *Conversation) Restore(messages []Message) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messages = append(c.messages[:0], messages...)
}

// TruncateTo removes all messages after length. It is used by command undo
// paths that record the conversation length before appending messages.
func (c *Conversation) TruncateTo(length int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if length < 0 || length > len(c.messages) {
		return fmt.Errorf("truncate conversation to %d: history length is %d", length, len(c.messages))
	}
	c.messages = c.messages[:length]
	return nil
}

// Messages is an alias for History. It exists to ease migration from
// generator's ConversationHistory which uses this method name.
func (c *Conversation) Messages() []Message {
	return c.History()
}

// AssembleMessages returns the full message list for a Chat call:
// system prompt (if non-empty) followed by all conversation messages.
// Useful in manual mode where the caller needs the complete input for
// an external Chat call.
func (c *Conversation) AssembleMessages() []Message {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.assembleMessagesLocked()
}

// Len returns the number of messages in the conversation (excluding
// the system prompt).
func (c *Conversation) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.messages)
}

// Reset clears the conversation history, allowing a fresh start within
// the same session. The system prompt and client binding are preserved.
func (c *Conversation) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messages = c.messages[:0]
}

// SystemPrompt returns the system prompt for this conversation.
func (c *Conversation) SystemPrompt() string {
	return c.systemPrompt
}

// SetSystemPrompt replaces the system prompt. Takes effect on the next
// Send call.
func (c *Conversation) SetSystemPrompt(prompt string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.systemPrompt = prompt
}

// assembleMessagesLocked builds the full message list for a Chat call:
// system prompt followed by all conversation messages. Caller must
// hold at least a read lock.
func (c *Conversation) assembleMessagesLocked() []Message {
	msgs := make([]Message, 0, 1+len(c.messages))
	if c.systemPrompt != "" {
		msgs = append(msgs, Message{Role: System, Content: c.systemPrompt})
	}
	msgs = append(msgs, c.messages...)
	return msgs
}
