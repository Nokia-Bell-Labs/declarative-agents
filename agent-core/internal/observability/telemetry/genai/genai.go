// Copyright (c) 2026 Nokia. All rights reserved.

// Package genai provides OpenTelemetry GenAI semantic convention constants
// and span-naming helpers.
//
// Reference: https://opentelemetry.io/docs/specs/semconv/gen-ai/gen-ai-spans/
// Reference: https://opentelemetry.io/docs/specs/semconv/gen-ai/gen-ai-agent-spans/
package genai

import "go.opentelemetry.io/otel/attribute"

// --- Attribute keys (gen_ai.*) ---

const (
	// Core identification.
	AttrOperationName = attribute.Key("gen_ai.operation.name")
	AttrProviderName  = attribute.Key("gen_ai.provider.name")

	// Model.
	AttrRequestModel  = attribute.Key("gen_ai.request.model")
	AttrResponseModel = attribute.Key("gen_ai.response.model")

	// Agent.
	AttrAgentName        = attribute.Key("gen_ai.agent.name")
	AttrAgentID          = attribute.Key("gen_ai.agent.id")
	AttrAgentVersion     = attribute.Key("gen_ai.agent.version")
	AttrAgentDescription = attribute.Key("gen_ai.agent.description")

	// Tool.
	AttrToolName        = attribute.Key("gen_ai.tool.name")
	AttrToolType        = attribute.Key("gen_ai.tool.type")
	AttrToolCallID      = attribute.Key("gen_ai.tool.call.id")
	AttrToolDescription = attribute.Key("gen_ai.tool.description")

	// Workflow.
	AttrWorkflowName = attribute.Key("gen_ai.workflow.name")

	// Conversation.
	AttrConversationID = attribute.Key("gen_ai.conversation.id")

	// Usage.
	AttrUsageInputTokens  = attribute.Key("gen_ai.usage.input_tokens")
	AttrUsageOutputTokens = attribute.Key("gen_ai.usage.output_tokens")

	// Request parameters.
	AttrRequestTemperature      = attribute.Key("gen_ai.request.temperature")
	AttrRequestSeed             = attribute.Key("gen_ai.request.seed")
	AttrRequestTopP             = attribute.Key("gen_ai.request.top_p")
	AttrRequestMaxTokens        = attribute.Key("gen_ai.request.max_tokens")
	AttrRequestStopSequences    = attribute.Key("gen_ai.request.stop_sequences")
	AttrRequestFrequencyPenalty = attribute.Key("gen_ai.request.frequency_penalty")
	AttrRequestPresencePenalty  = attribute.Key("gen_ai.request.presence_penalty")

	// Response.
	AttrResponseFinishReasons = attribute.Key("gen_ai.response.finish_reasons")
	AttrResponseID            = attribute.Key("gen_ai.response.id")

	// Output.
	AttrOutputType = attribute.Key("gen_ai.output.type")

	// Content (opt-in, may contain sensitive data).
	AttrInputMessages      = attribute.Key("gen_ai.input.messages")
	AttrOutputMessages     = attribute.Key("gen_ai.output.messages")
	AttrSystemInstructions = attribute.Key("gen_ai.system_instructions")
	AttrToolDefinitions    = attribute.Key("gen_ai.tool.definitions")
	AttrToolCallArguments  = attribute.Key("gen_ai.tool.call.arguments")
	AttrToolCallResult     = attribute.Key("gen_ai.tool.call.result")

	// Server.
	AttrServerAddress = attribute.Key("server.address")
	AttrServerPort    = attribute.Key("server.port")

	// Error.
	AttrErrorType = attribute.Key("error.type")
)

// --- Operation names ---

const (
	OpChat           = "chat"
	OpCreateAgent    = "create_agent"
	OpInvokeAgent    = "invoke_agent"
	OpExecuteTool    = "execute_tool"
	OpInvokeWorkflow = "invoke_workflow"
	OpEmbeddings     = "embeddings"
	OpRetrieval      = "retrieval"
	OpTextCompletion = "text_completion"
)

// --- Provider names ---

const (
	ProviderOllama    = "ollama"
	ProviderOpenAI    = "openai"
	ProviderAnthropic = "anthropic"
	ProviderDeepSeek  = "deepseek"
)

// --- Tool types ---

const (
	ToolTypeFunction  = "function"
	ToolTypeExtension = "extension"
	ToolTypeDatastore = "datastore"
)

// --- Output types ---

const (
	OutputText  = "text"
	OutputJSON  = "json"
	OutputImage = "image"
)
