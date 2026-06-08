// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"testing"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/llm"
)

func TestConversationStore_GetCreatesNew(t *testing.T) {
	cs := NewConversationStore()
	c := cs.Get("plan")
	if c == nil {
		t.Fatal("expected non-nil conversation")
	}
	if c.Len() != 0 {
		t.Errorf("new conversation should have 0 messages, got %d", c.Len())
	}
}

func TestConversationStore_GetReturnsSame(t *testing.T) {
	cs := NewConversationStore()
	c1 := cs.Get("code")
	c1.Append(llm.Message{Role: llm.User, Content: "hello"})

	c2 := cs.Get("code")
	if c2.Len() != 1 {
		t.Errorf("expected same instance with 1 message, got %d", c2.Len())
	}
}

func TestConversationStore_SeparateHistories(t *testing.T) {
	cs := NewConversationStore()
	plan := cs.Get("plan")
	code := cs.Get("code")

	plan.Append(llm.Message{Role: llm.User, Content: "plan this"})
	plan.Append(llm.Message{Role: llm.User, Content: "more planning"})
	code.Append(llm.Message{Role: llm.User, Content: "write code"})

	if plan.Len() != 2 {
		t.Errorf("plan should have 2 messages, got %d", plan.Len())
	}
	if code.Len() != 1 {
		t.Errorf("code should have 1 message, got %d", code.Len())
	}
}

func TestConversationStore_Reset(t *testing.T) {
	cs := NewConversationStore()
	c := cs.Get("test")
	c.Append(llm.Message{Role: llm.User, Content: "hello"})
	cs.Reset("test")

	if c.Len() != 0 {
		t.Errorf("expected 0 messages after reset, got %d", c.Len())
	}
}

func TestConversationStore_ResetMissing(t *testing.T) {
	cs := NewConversationStore()
	cs.Reset("nonexistent") // should not panic
}

func TestConversationStore_Names(t *testing.T) {
	cs := NewConversationStore()
	cs.Get("alpha")
	cs.Get("beta")

	names := cs.Names()
	if len(names) != 2 {
		t.Errorf("expected 2 names, got %d", len(names))
	}
}
