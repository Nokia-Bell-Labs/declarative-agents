// Copyright (c) 2026 Nokia. All rights reserved.

package genai

import "go.opentelemetry.io/otel/attribute"

// --- Span name builders ---

// InferenceSpanName returns "chat {model}" (or just "chat" if model is empty).
func InferenceSpanName(model string) string {
	if model == "" {
		return OpChat
	}
	return OpChat + " " + model
}

// AgentSpanName returns "invoke_agent {name}" (or just "invoke_agent").
func AgentSpanName(name string) string {
	if name == "" {
		return OpInvokeAgent
	}
	return OpInvokeAgent + " " + name
}

// ToolSpanName returns "execute_tool {name}".
func ToolSpanName(name string) string {
	if name == "" {
		return OpExecuteTool
	}
	return OpExecuteTool + " " + name
}

// WorkflowSpanName returns "invoke_workflow {name}".
func WorkflowSpanName(name string) string {
	if name == "" {
		return OpInvokeWorkflow
	}
	return OpInvokeWorkflow + " " + name
}

// --- Attribute builders ---

// InferenceAttrs returns the required and recommended attributes for an
// inference (chat) span at creation time.
func InferenceAttrs(provider, model, serverAddr string) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		AttrOperationName.String(OpChat),
		AttrProviderName.String(provider),
	}
	if model != "" {
		attrs = append(attrs, AttrRequestModel.String(model))
	}
	if serverAddr != "" {
		attrs = append(attrs, AttrServerAddress.String(serverAddr))
	}
	return attrs
}

// AgentAttrs returns the required and conditional attributes for an
// invoke_agent span at creation time.
func AgentAttrs(name, version, provider, model string) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		AttrOperationName.String(OpInvokeAgent),
	}
	if provider != "" {
		attrs = append(attrs, AttrProviderName.String(provider))
	}
	if name != "" {
		attrs = append(attrs, AttrAgentName.String(name))
	}
	if version != "" {
		attrs = append(attrs, AttrAgentVersion.String(version))
	}
	if model != "" {
		attrs = append(attrs, AttrRequestModel.String(model))
	}
	return attrs
}

// ToolAttrs returns the required and recommended attributes for an
// execute_tool span at creation time.
func ToolAttrs(name, toolType string) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		AttrOperationName.String(OpExecuteTool),
		AttrToolName.String(name),
	}
	if toolType != "" {
		attrs = append(attrs, AttrToolType.String(toolType))
	}
	return attrs
}

// WorkflowAttrs returns the required and conditional attributes for an
// invoke_workflow span at creation time.
func WorkflowAttrs(name string) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		AttrOperationName.String(OpInvokeWorkflow),
	}
	if name != "" {
		attrs = append(attrs, AttrWorkflowName.String(name))
	}
	return attrs
}

// UsageAttrs returns token usage attributes to set on a span after completion.
func UsageAttrs(inputTokens, outputTokens int) []attribute.KeyValue {
	return []attribute.KeyValue{
		AttrUsageInputTokens.Int(inputTokens),
		AttrUsageOutputTokens.Int(outputTokens),
	}
}

// ErrorAttrs returns the error.type attribute for recording errors on spans.
func ErrorAttrs(errType string) []attribute.KeyValue {
	if errType == "" {
		errType = "_OTHER"
	}
	return []attribute.KeyValue{
		AttrErrorType.String(errType),
	}
}
