// Copyright (c) 2026 Nokia. All rights reserved.

package llm

import (
	"fmt"
	"sync"
)

// Conversation owns the append-only model history used by invoke_llm.
type Conversation struct {
	turnMu   sync.Mutex
	mu       sync.RWMutex
	messages []Message
}

// NewConversation creates empty model history. The legacy arguments remain
// accepted while callers migrate; inference and prompt assembly live in the
// invoke_llm boundary rather than this state container.
func NewConversation(_ Client, _ string, _ ChatOptions) *Conversation {
	return &Conversation{}
}

// Append adds a message to the conversation history without triggering
// a Chat call. This supports the manual/append-only pattern where the
// caller builds the history independently and invokes Chat externally.
//
// Use Send for the managed pattern (auto-calls Chat). Use Append when
// assembling messages from multiple sources before a single Chat call.
func (c *Conversation) Append(msg Message) {
	c.turnMu.Lock()
	defer c.turnMu.Unlock()

	c.mu.Lock()
	defer c.mu.Unlock()
	c.messages = append(c.messages, msg)
}

// History returns a point-in-time copy of the conversation messages
// in insertion order, oldest first.
func (c *Conversation) History() []Message {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]Message, len(c.messages))
	copy(out, c.messages)
	return out
}

// Snapshot returns a copy of the current completed conversation history.
func (c *Conversation) Snapshot() []Message {
	c.turnMu.Lock()
	defer c.turnMu.Unlock()
	return c.History()
}

// Restore replaces the conversation history with the provided messages.
// The system prompt, client, and options are preserved.
func (c *Conversation) Restore(messages []Message) {
	c.turnMu.Lock()
	defer c.turnMu.Unlock()

	c.mu.Lock()
	defer c.mu.Unlock()
	c.messages = append(c.messages[:0], messages...)
}

// TruncateTo removes all messages after length. It is used by command undo
// paths that record the conversation length before appending messages.
func (c *Conversation) TruncateTo(length int) error {
	c.turnMu.Lock()
	defer c.turnMu.Unlock()

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

// Len returns the number of messages in the conversation (excluding
// the system prompt).
func (c *Conversation) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.messages)
}

// Reset clears the conversation history.
func (c *Conversation) Reset() {
	c.turnMu.Lock()
	defer c.turnMu.Unlock()

	c.mu.Lock()
	defer c.mu.Unlock()
	c.messages = c.messages[:0]
}
