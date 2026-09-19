// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	modelllm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/telemetry/genai"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/credentials"
)

// invoke_llm as a template method over a chat dialect (srd058 R3). The fixture
// library answers like Cohere v2: bearer auth, typed content blocks, nested
// usage.

const fixtureToken = "fixture-secret-7f3a"

func fixtureDialect(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "..", "..", "testdata", "providers", "fixture", "chat-dialect.yaml"))
	require.NoError(t, err)
	return path
}

func dialectBuilder(t *testing.T, dialect, url string, resolver credentials.Resolver, tracer tracing.Tracer) *InvokeLLMBuilder {
	t.Helper()
	def := catalog.ToolDef{Name: "invoke_llm", Config: map[string]interface{}{
		"dialect": dialect, "provider_url": url,
		"model": "fixture-large", "manifest_state": "Composing",
	}}
	builder, err := NewInvokeLLMBuilder(def, InvokeLLMFactoryDeps{
		History:  modelllm.NewConversation(nil, "", modelllm.ChatOptions{}),
		Registry: core.NewRegistry(), Tracer: tracer, Ctx: context.Background(),
		Credentials: resolver,
	})
	require.NoError(t, err)
	return builder
}

func TestInvokeLLMRunsTheDialectPipeline(t *testing.T) {
	t.Parallel()
	var request map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/chat", r.URL.Path)
		require.Equal(t, "Bearer "+fixtureToken, r.Header.Get("Authorization"))
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		_, _ = w.Write([]byte(`{"message":{"content":[{"type":"text","text":"hello"}]},
			"usage":{"tokens":{"input_tokens":9,"output_tokens":2}}}`))
	}))
	defer server.Close()
	recorder := tracing.NewRecordingTracer()
	builder := dialectBuilder(t, fixtureDialect(t), server.URL,
		credentials.Static{"FIXTURE_API_KEY": fixtureToken}, recorder)

	result := builder.Build(core.Result{State: "Composing", Output: "hi"}).Execute()

	require.Equal(t, core.LLMResponded, result.Signal)
	require.Equal(t, "hello", result.Output)
	require.Equal(t, 9, result.Cost.TokensIn)
	require.Equal(t, 2, result.Cost.TokensOut)
	require.Equal(t, "fixture", builder.ProviderName, "spans name the dialect's provider")

	require.Equal(t, "fixture-large", request["model"])
	require.Equal(t, false, request["stream"])
	require.Equal(t, float64(42), request["seed"], "the deterministic seed default reaches the body")
	require.Equal(t, map[string]interface{}{}, request["options"], "an unset num_ctx is omitted, not null")
	messages, ok := request["messages"].([]interface{})
	require.True(t, ok, "the conversation is placed as an array")
	last := messages[len(messages)-1].(map[string]interface{})
	require.Equal(t, map[string]interface{}{"role": "user", "content": "hi"}, last)

	history := builder.History.Snapshot()
	require.Equal(t, "hello", history[len(history)-1].Content, "the reply is appended to history")

	require.NotEmpty(t, recorder.Spans)
	span := recorder.Spans[0]
	require.Equal(t, genai.InferenceSpanName("fixture-large"), span.Name)
	require.Equal(t, "fixture", span.Attrs[string(genai.AttrProviderName)])
	require.Equal(t, int64(9), span.SetAttrs[string(genai.AttrUsageInputTokens)])
}

func TestInvokeLLMRejectsProviderAndDialect(t *testing.T) {
	t.Parallel()
	_, err := DecodeInvokeLLMConfig(catalog.ToolDef{Config: map[string]interface{}{
		"provider": "ollama", "dialect": "/opt/providers/chat-dialect.yaml",
		"model": "m", "manifest_state": "Composing",
	}})
	require.ErrorContains(t, err, "names both provider")

	dialectOnly, err := DecodeInvokeLLMConfig(catalog.ToolDef{Config: map[string]interface{}{
		"dialect": "/opt/providers/chat-dialect.yaml", "model": "m", "manifest_state": "Composing",
	}})
	require.NoError(t, err)
	require.Empty(t, dialectOnly.Provider, "a dialect config is not defaulted to a provider")
	_, err = NewInvokeLLMBuilder(catalog.ToolDef{Config: map[string]interface{}{
		"dialect": fixtureDialect(t), "model": "m", "manifest_state": "Composing",
	}}, InvokeLLMFactoryDeps{})
	require.ErrorContains(t, err, "requires provider_url")
}

func TestDialectCredentialResolvedAtCallTime(t *testing.T) {
	t.Parallel()
	var sent atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sent.Store(r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{"message":{"content":[{"type":"text","text":"ok"}]}}`))
	}))
	defer server.Close()
	resolver := credentials.Static{}
	builder := dialectBuilder(t, fixtureDialect(t), server.URL, resolver, tracing.NoopTracer{})
	resolver["FIXTURE_API_KEY"] = fixtureToken

	result := builder.Build(core.Result{State: "Composing", Output: "hi"}).Execute()

	require.Equal(t, core.LLMResponded, result.Signal)
	require.Equal(t, "Bearer "+fixtureToken, sent.Load(), "a credential set after build is the one sent")
}

func TestDialectMissingCredentialFailsBeforeNetwork(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer server.Close()
	builder := dialectBuilder(t, fixtureDialect(t), server.URL, credentials.Static{}, tracing.NoopTracer{})

	result := builder.Build(core.Result{State: "Composing", Output: "hi"}).Execute()

	require.Equal(t, core.CommandError, result.Signal)
	require.ErrorContains(t, result.Err, `"FIXTURE_API_KEY" is not resolved`)
	require.Zero(t, calls.Load())
}

func TestDialectCredentialAbsentFromDiagnostics(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"message":"slow down, key ` + fixtureToken + `"}`))
	}))
	defer server.Close()
	recorder := tracing.NewRecordingTracer()
	builder := dialectBuilder(t, fixtureDialect(t), server.URL,
		credentials.Static{"FIXTURE_API_KEY": fixtureToken}, recorder)

	result := builder.Build(core.Result{State: "Composing", Output: "hi"}).Execute()

	require.Equal(t, core.CommandError, result.Signal, "a mapped failure still emits CommandError (R3.3)")
	require.ErrorContains(t, result.Err, "status 429 (ProviderThrottled)")
	require.NotContains(t, result.Err.Error(), fixtureToken)
	require.NotContains(t, result.Output, fixtureToken)
	for _, span := range recorder.Spans {
		for _, attrs := range []map[string]interface{}{span.Attrs, span.SetAttrs} {
			for _, value := range attrs {
				require.NotContains(t, strings.ToLower(toString(value)), fixtureToken)
			}
		}
	}
	require.Equal(t, "429", recorder.Spans[0].SetAttrs[string(genai.AttrErrorType)])
}

func TestDialectLibraryParserProfileResolves(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(fixtureDialect(t))
	require.NoError(t, err)
	library := string(data) + `parser_profiles:
- name: fixture
  match_prefixes: ["fixture-"]
  envelope: {open: "<call>", close: "</call>"}
  extraction_pipeline:
  - extract_envelope: {open: "<call>", close: "</call>"}
`
	path := filepath.Join(t.TempDir(), "chat-dialect.yaml")
	require.NoError(t, os.WriteFile(path, []byte(library), 0o644))
	resolved := &ResolvedModel{}
	def := catalog.ToolDef{Name: "invoke_llm", Config: map[string]interface{}{
		"dialect": path, "provider_url": "http://127.0.0.1:1", "model": "fixture-large", "manifest_state": "Composing",
	}}

	_, err = NewInvokeLLMBuilder(def, InvokeLLMFactoryDeps{OnResolved: applyResolved(resolved)})

	require.NoError(t, err)
	require.Equal(t, `{"a":1}`, resolved.Parser.ExtractToolCall(`<call>{"a":1}</call>`),
		"the model resolves to the library's profile by prefix")
	named, err := parseResponseParser(catalog.ParseResponseConfig{ResponseProfile: "fixture"}, resolved)
	require.NoError(t, err)
	require.Equal(t, `{"a":1}`, named.ExtractToolCall(`<call>{"a":1}</call>`),
		"a parse word naming the library profile finds it")
}

func toString(value interface{}) string {
	data, _ := json.Marshal(value)
	return string(data)
}
