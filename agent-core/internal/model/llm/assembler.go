// Copyright (c) 2026 Nokia. All rights reserved.

package llm

import (
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/model/prompt"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
)

// DefaultAssembler implements PromptAssembler with the standard prompt
// structure: system message (rendered from PromptData with envelope
// config and tool manifest), followed by conversation history.
type DefaultAssembler struct {
	Prompt prompt.Prompt
	Parser ResponseParser
}

func (a *DefaultAssembler) AssembleMessages(conv *Conversation, registry *core.Registry, state core.State) []Message {
	var messages []Message

	parser := a.Parser
	if parser == nil {
		parser = DefaultProfile()
	}

	envelope, strict := parser.EnvelopeConfig()
	manifest := registry.Manifest(state)

	data := prompt.PromptData{
		Role:         a.Prompt.Role,
		Task:         a.Prompt.Task,
		Constraints:  a.Prompt.Constraints,
		OutputFormat: a.Prompt.OutputFormat,
		Envelope:     envelope,
		StrictFormat: strict,
	}
	if len(manifest) > 0 {
		data.ToolManifest = prompt.SerializeManifest(manifest)
	}

	messages = append(messages, Message{Role: System, Content: prompt.RenderSystemPrompt(data)})
	messages = append(messages, conv.Messages()...)

	return messages
}

var _ PromptAssembler = (*DefaultAssembler)(nil)
