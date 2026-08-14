// Copyright (c) 2026 Nokia. All rights reserved.

package llm

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	"go.opentelemetry.io/otel/attribute"

	modelllm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/telemetry/genai"
)

// ConversationReferenceProvider exposes an optional durable reference to the
// authoritative conversation snapshot without coupling LLM tools to checkpoint
// implementations.
type ConversationReferenceProvider interface {
	ConversationReference() (string, bool)
}

func (c *invokeLLMCmd) recordInputCapture(messages []modelllm.Message) {
	c.tracer.SetAttributes(genai.AttrSystemPromptHash.String(renderedSystemPromptHash(messages)))
	if ref, ok := c.conversationReference(); ok {
		c.tracer.SetAttributes(genai.AttrConversationRef.String(ref))
	}
	switch c.captureLevel {
	case CaptureDelta:
		c.setJSONCapture(
			genai.AttrInputDelta,
			modelllm.Message{Role: modelllm.User, Content: c.userMessage},
		)
	case CaptureFull:
		c.setJSONCapture(genai.AttrInputMessages, messages)
	}
}

func (c *invokeLLMCmd) recordOutputCapture(content string) {
	message := modelllm.Message{Role: modelllm.Assistant, Content: content}
	switch c.captureLevel {
	case CaptureDelta:
		c.setJSONCapture(genai.AttrOutputDelta, message)
	case CaptureFull:
		c.setJSONCapture(genai.AttrOutputMessages, []modelllm.Message{message})
	}
}

func (c *invokeLLMCmd) setJSONCapture(key attribute.Key, value interface{}) {
	data, err := json.Marshal(value)
	if err != nil {
		c.tracer.Event("capture.serialization_failed",
			attribute.String("attribute", string(key)),
			attribute.String("error", err.Error()))
		return
	}
	c.tracer.SetAttributes(key.String(string(data)))
}

func (c *invokeLLMCmd) conversationReference() (string, bool) {
	if c.conversationRefProvider == nil {
		return "", false
	}
	ref, ok := c.conversationRefProvider.ConversationReference()
	ref = strings.TrimSpace(ref)
	return ref, ok && ref != ""
}

func renderedSystemPromptHash(messages []modelllm.Message) string {
	content := ""
	for _, message := range messages {
		if message.Role == modelllm.System {
			content = message.Content
			break
		}
	}
	digest := sha256.Sum256([]byte(content))
	return fmt.Sprintf("sha256:%x", digest)
}
