// Copyright (c) 2026 Nokia. All rights reserved.

package llm

import (
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/model/prompt"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
)

// ResponseParser extracts structured tool calls from raw LLM output.
// Implementations handle model-specific formatting: envelope tags,
// code fences, thinking blocks, native tokens, etc.
//
// Generator supplies a YAML-profile-based implementation. Planner may
// use a simpler parser for bare JSON responses.
type ResponseParser interface {
	// ExtractToolCall strips model-specific wrappers from raw LLM
	// output and returns the JSON tool call string.
	ExtractToolCall(raw string) string

	// EnvelopeConfig returns the envelope tags for wrapping tool calls
	// in the system prompt, and whether strict format (no extra text
	// outside the envelope) should be enforced. A nil *Envelope means
	// bare JSON with no wrapping tags.
	EnvelopeConfig() (envelope *prompt.Envelope, strictFormat bool)
}

// PromptAssembler builds the complete messages array for an LLM Chat
// call. This is where agents inject domain-specific prompt structure:
// system prompt rendering, tool manifest formatting, and any
// additional context the agent needs in the prompt.
//
// Generator assembles Role+Task+Constraints+OutputFormat+ToolManifest.
// Planner assembles a different prompt structure for planning tasks.
type PromptAssembler interface {
	// AssembleMessages returns the full ordered message list for a
	// Chat call: system prompt, any injected context, and the
	// conversation history. The registry and state are provided so
	// the assembler can build a state-filtered tool manifest.
	AssembleMessages(conv *Conversation, registry *core.Registry, state core.State) []Message
}
