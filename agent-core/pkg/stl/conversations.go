// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/llm"
)

// ConversationStore manages named conversation histories so that
// multiple LLM tool instances can maintain separate contexts.
type ConversationStore struct {
	convs map[string]*llm.Conversation
}

// NewConversationStore creates an empty store.
func NewConversationStore() *ConversationStore {
	return &ConversationStore{convs: make(map[string]*llm.Conversation)}
}

// Get returns the conversation for the given name, creating one if needed.
// New conversations are created with no client or system prompt — they
// serve as bare history stores for InvokeLLMBuilder.
func (cs *ConversationStore) Get(name string) *llm.Conversation {
	if c, ok := cs.convs[name]; ok {
		return c
	}
	c := llm.NewConversation(nil, "", llm.ChatOptions{})
	cs.convs[name] = c
	return c
}

// Reset clears the conversation for the given name.
func (cs *ConversationStore) Reset(name string) {
	if c, ok := cs.convs[name]; ok {
		c.Reset()
	}
}

// Names returns all conversation names.
func (cs *ConversationStore) Names() []string {
	names := make([]string, 0, len(cs.convs))
	for n := range cs.convs {
		names = append(names, n)
	}
	return names
}
