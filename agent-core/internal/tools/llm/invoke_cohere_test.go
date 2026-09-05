// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	modelllm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	"github.com/stretchr/testify/require"
)

func TestNewInvokeLLMBuilder_CohereNoProbe(t *testing.T) {
	t.Parallel()
	def := catalog.ToolDef{Name: "invoke_llm", Config: map[string]interface{}{
		"provider": "cohere", "provider_url": "http://127.0.0.1:1",
		"model": "command-r7b-12-2024", "manifest_state": "Composing",
		"response_profile": "cohere",
	}}

	builder, err := NewInvokeLLMBuilder(def, InvokeLLMFactoryDeps{Ctx: context.Background()})

	require.NoError(t, err)
	require.NotNil(t, builder)
	require.Equal(t, "cohere", builder.ProviderName)
}

func TestInvokeLLM_CohereConfiguredProvider(t *testing.T) {
	t.Setenv("COHERE_API_KEY", "test-key")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		require.Equal(t, "/v2/chat", request.URL.Path)
		require.Equal(t, "Bearer test-key", request.Header.Get("Authorization"))
		var body map[string]interface{}
		require.NoError(t, json.NewDecoder(request.Body).Decode(&body))
		require.Equal(t, "command-r7b-12-2024", body["model"])
		_, _ = w.Write([]byte(`{
			"message":{"content":[{"type":"text","text":"{\"names\":[\"rag0\"]}"}]},
			"usage":{"tokens":{"input_tokens":12,"output_tokens":4}}
		}`))
	}))
	defer server.Close()
	def := catalog.ToolDef{Name: "select_sources", Config: map[string]interface{}{
		"provider": "cohere", "provider_url": server.URL,
		"model": "command-r7b-12-2024", "manifest_state": "SelectingSources",
		"response_profile": "cohere",
	}}
	builder, err := NewInvokeLLMBuilder(def, InvokeLLMFactoryDeps{
		History:  modelllm.NewConversation(nil, "", modelllm.ChatOptions{}),
		Registry: core.NewRegistry(), Tracer: tracing.NoopTracer{}, Ctx: context.Background(),
	})
	require.NoError(t, err)

	result := builder.Build(core.Result{State: "SelectingSources", Output: "choose sources"}).Execute()
	require.Equal(t, core.LLMResponded, result.Signal)
	require.Equal(t, `{"names":["rag0"]}`, result.Output)
	require.Equal(t, 12, result.Cost.TokensIn)
	require.Equal(t, 4, result.Cost.TokensOut)
}

func TestInvokeLLM_CohereCommandError(t *testing.T) {
	t.Run("missing credential", func(t *testing.T) {
		t.Setenv("COHERE_API_KEY", "")
		var calls int
		server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
		defer server.Close()
		builder := cohereInvokeBuilder(t, server.URL)

		result := builder.Build(core.Result{State: "Composing", Output: "prompt"}).Execute()
		require.Equal(t, core.CommandError, result.Signal)
		require.ErrorContains(t, result.Err, "COHERE_API_KEY is not resolved")
		require.Zero(t, calls)
	})
	t.Run("unreachable service", func(t *testing.T) {
		t.Setenv("COHERE_API_KEY", "key")
		builder := cohereInvokeBuilder(t, "http://127.0.0.1:1")

		result := builder.Build(core.Result{State: "Composing", Output: "prompt"}).Execute()
		require.Equal(t, core.CommandError, result.Signal)
		require.ErrorContains(t, result.Err, "cohere chat request failed")
	})
}

func cohereInvokeBuilder(t *testing.T, providerURL string) *InvokeLLMBuilder {
	t.Helper()
	def := catalog.ToolDef{Name: "invoke_llm", Config: map[string]interface{}{
		"provider": "cohere", "provider_url": providerURL,
		"model": "command-r7b-12-2024", "manifest_state": "Composing",
		"response_profile": "cohere",
	}}
	builder, err := NewInvokeLLMBuilder(def, InvokeLLMFactoryDeps{
		History:  modelllm.NewConversation(nil, "", modelllm.ChatOptions{}),
		Registry: core.NewRegistry(), Tracer: tracing.NoopTracer{}, Ctx: context.Background(),
	})
	require.NoError(t, err)
	return builder
}

func TestInvokeLLMProviderSelection(t *testing.T) {
	t.Parallel()
	omitted, err := DecodeInvokeLLMConfig(catalog.ToolDef{Config: map[string]interface{}{
		"model": "qwen", "manifest_state": "Composing",
	}})
	require.NoError(t, err)
	require.Equal(t, "ollama", omitted.Provider)
	require.Equal(t, "http://localhost:11434", omitted.ProviderURL)

	cohereConfig, err := DecodeInvokeLLMConfig(catalog.ToolDef{Config: map[string]interface{}{
		"provider": "cohere", "model": "command-r7b-12-2024", "manifest_state": "Composing",
	}})
	require.NoError(t, err)
	require.Empty(t, cohereConfig.ProviderURL)
	_, _, err = newLLMClient(cohereConfig, tracing.NoopTracer{})
	require.ErrorContains(t, err, `provider "cohere" requires provider_url`)

	unknown := cohereConfig
	unknown.Provider = "unknown"
	unknown.ProviderURL = "http://provider.invalid"
	_, _, err = newLLMClient(unknown, tracing.NoopTracer{})
	require.ErrorContains(t, err, `unsupported invoke_llm provider "unknown"`)
}
