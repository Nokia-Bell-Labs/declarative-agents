// Copyright (c) 2026 Nokia. All rights reserved.

package genai

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/attribute"
)

func TestInferenceSpanName(t *testing.T) {
	assert.Equal(t, "chat qwen2.5-coder:14b", InferenceSpanName("qwen2.5-coder:14b"))
	assert.Equal(t, "chat", InferenceSpanName(""))
}

func TestAgentSpanName(t *testing.T) {
	assert.Equal(t, "invoke_agent generator", AgentSpanName("generator"))
	assert.Equal(t, "invoke_agent planner", AgentSpanName("planner"))
	assert.Equal(t, "invoke_agent", AgentSpanName(""))
}

func TestToolSpanName(t *testing.T) {
	assert.Equal(t, "execute_tool read", ToolSpanName("read"))
	assert.Equal(t, "execute_tool build", ToolSpanName("build"))
	assert.Equal(t, "execute_tool", ToolSpanName(""))
}

func TestWorkflowSpanName(t *testing.T) {
	assert.Equal(t, "invoke_workflow planner", WorkflowSpanName("planner"))
	assert.Equal(t, "invoke_workflow", WorkflowSpanName(""))
}

func TestInferenceAttrs(t *testing.T) {
	attrs := InferenceAttrs("ollama", "qwen2.5-coder:14b", "localhost:11434")

	m := attrMap(attrs)
	assert.Equal(t, "chat", m["gen_ai.operation.name"])
	assert.Equal(t, "ollama", m["gen_ai.provider.name"])
	assert.Equal(t, "qwen2.5-coder:14b", m["gen_ai.request.model"])
	assert.Equal(t, "localhost:11434", m["server.address"])
}

func TestInferenceAttrsMinimal(t *testing.T) {
	attrs := InferenceAttrs("ollama", "", "")

	m := attrMap(attrs)
	assert.Equal(t, "chat", m["gen_ai.operation.name"])
	assert.Equal(t, "ollama", m["gen_ai.provider.name"])
	_, hasModel := m["gen_ai.request.model"]
	assert.False(t, hasModel, "model should be omitted when empty")
}

func TestAgentAttrs(t *testing.T) {
	attrs := AgentAttrs("generator", "v0.20260605.0", "ollama", "qwen2.5-coder:14b")

	m := attrMap(attrs)
	assert.Equal(t, "invoke_agent", m["gen_ai.operation.name"])
	assert.Equal(t, "ollama", m["gen_ai.provider.name"])
	assert.Equal(t, "generator", m["gen_ai.agent.name"])
	assert.Equal(t, "v0.20260605.0", m["gen_ai.agent.version"])
	assert.Equal(t, "qwen2.5-coder:14b", m["gen_ai.request.model"])
}

func TestToolAttrs(t *testing.T) {
	attrs := ToolAttrs("read", "function")

	m := attrMap(attrs)
	assert.Equal(t, "execute_tool", m["gen_ai.operation.name"])
	assert.Equal(t, "read", m["gen_ai.tool.name"])
	assert.Equal(t, "function", m["gen_ai.tool.type"])
}

func TestWorkflowAttrs(t *testing.T) {
	attrs := WorkflowAttrs("planner")

	m := attrMap(attrs)
	assert.Equal(t, "invoke_workflow", m["gen_ai.operation.name"])
	assert.Equal(t, "planner", m["gen_ai.workflow.name"])
}

func TestUsageAttrs(t *testing.T) {
	attrs := UsageAttrs(100, 180)

	m := attrMap(attrs)
	assert.Equal(t, "100", m["gen_ai.usage.input_tokens"])
	assert.Equal(t, "180", m["gen_ai.usage.output_tokens"])
}

func TestErrorAttrs(t *testing.T) {
	attrs := ErrorAttrs("timeout")
	m := attrMap(attrs)
	assert.Equal(t, "timeout", m["error.type"])

	attrs = ErrorAttrs("")
	m = attrMap(attrs)
	assert.Equal(t, "_OTHER", m["error.type"])
}

func attrMap(attrs []attribute.KeyValue) map[string]string {
	m := make(map[string]string)
	for _, a := range attrs {
		m[string(a.Key)] = a.Value.Emit()
	}
	return m
}
