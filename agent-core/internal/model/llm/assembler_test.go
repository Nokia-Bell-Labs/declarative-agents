// Copyright (c) 2026 Nokia. All rights reserved.

package llm

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/prompt"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func TestDefaultAssembler_NilParserUsesDefault(t *testing.T) {
	reg := core.NewRegistry()
	conv := NewConversation(nil, "", ChatOptions{})

	asm := &DefaultAssembler{
		Prompt: prompt.Prompt{Role: "You are a test agent."},
	}

	msgs := asm.AssembleMessages(conv, reg, core.State("Idle"))
	require.NotEmpty(t, msgs)
	assert.Equal(t, System, msgs[0].Role)
	assert.Contains(t, msgs[0].Content, "You are a test agent.")
}

func TestDefaultAssembler_RendersManifest(t *testing.T) {
	reg := core.NewRegistry()
	reg.Register(core.ToolSpec{
		Name:        "read_file",
		Description: "Read a file from disk.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`),
		Visibility:  core.External,
	}, nil)

	conv := NewConversation(nil, "", ChatOptions{})

	asm := &DefaultAssembler{
		Prompt: prompt.Prompt{
			Role: "You are a coding assistant.",
			Task: "Help the user write code.",
		},
	}

	msgs := asm.AssembleMessages(conv, reg, core.State("Idle"))
	require.NotEmpty(t, msgs)

	sysContent := msgs[0].Content
	assert.Contains(t, sysContent, "read_file")
	assert.Contains(t, sysContent, "Read a file from disk.")
}

func TestDefaultAssembler_SuppressManifestOmitsTools(t *testing.T) {
	reg := core.NewRegistry()
	reg.Register(core.ToolSpec{
		Name:        "read_file",
		Description: "Read a file from disk.",
		InputSchema: json.RawMessage(`{"type":"object"}`),
		Visibility:  core.External,
	}, nil)

	conv := NewConversation(nil, "", ChatOptions{})
	asm := &DefaultAssembler{
		Prompt:           prompt.Prompt{Role: "You answer directly."},
		SuppressManifest: true,
	}

	msgs := asm.AssembleMessages(conv, reg, core.State("Idle"))
	require.NotEmpty(t, msgs)
	// The tool manifest is omitted so an answer-only word produces a final answer
	// instead of a tool call, even though the state has external tools available.
	assert.NotContains(t, msgs[0].Content, "read_file")
	assert.Contains(t, msgs[0].Content, "You answer directly.")
}

func TestDefaultAssembler_AppendsConversationMessages(t *testing.T) {
	reg := core.NewRegistry()
	conv := NewConversation(nil, "", ChatOptions{})
	conv.Append(Message{Role: User, Content: "Hello"})
	conv.Append(Message{Role: Assistant, Content: "Hi there"})

	asm := &DefaultAssembler{
		Prompt: prompt.Prompt{Role: "Agent role."},
	}

	msgs := asm.AssembleMessages(conv, reg, core.State("Idle"))
	require.Len(t, msgs, 3)
	assert.Equal(t, System, msgs[0].Role)
	assert.Equal(t, User, msgs[1].Role)
	assert.Equal(t, "Hello", msgs[1].Content)
	assert.Equal(t, Assistant, msgs[2].Role)
	assert.Equal(t, "Hi there", msgs[2].Content)
}

func TestDefaultAssembler_WithExplicitParser(t *testing.T) {
	reg := core.NewRegistry()
	conv := NewConversation(nil, "", ChatOptions{})

	profile := newYAMLProfile(ProfileSpec{
		ProfileName:  "test",
		Envelope:     nil,
		StrictFormat: true,
	})

	asm := &DefaultAssembler{
		Prompt: prompt.Prompt{Role: "Strict agent."},
		Parser: profile,
	}

	msgs := asm.AssembleMessages(conv, reg, core.State("Idle"))
	require.NotEmpty(t, msgs)
	// With nil envelope and strict=true, the system prompt should not
	// mention envelope tags but should still render the role.
	assert.True(t, strings.Contains(msgs[0].Content, "Strict agent."))
}
