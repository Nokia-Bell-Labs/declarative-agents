// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	modelllm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/credentials"
)

// The srd058 R5.3 conformance suite: a third provider, OpenAI-shaped, added as
// a directory of YAML under testdata, runs the unchanged invoke_llm word with
// no Go written for it. Its chat, its own parser profile, its credential, and
// its failure taxonomy all come from chat-dialect.yaml.

func syntheticDialect(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "..", "..", "testdata", "providers", "openai-shaped", "chat-dialect.yaml"))
	require.NoError(t, err)
	return path
}

func syntheticBuilder(t *testing.T, url string, resolved *ResolvedModel) *InvokeLLMBuilder {
	t.Helper()
	builder, err := NewInvokeLLMBuilder(catalog.ToolDef{Name: "invoke_llm", Config: map[string]interface{}{
		"dialect": syntheticDialect(t), "provider_url": url,
		"model": "gpt-synthetic", "manifest_state": "Composing", "system_prompt": "Answer briefly.",
	}}, InvokeLLMFactoryDeps{
		History:  modelllm.NewConversation(nil, "", modelllm.ChatOptions{}),
		Registry: core.NewRegistry(), Tracer: tracing.NoopTracer{}, Ctx: context.Background(),
		Credentials: credentials.Static{"OPENAI_API_KEY": "synthetic-key"},
		OnResolved:  applyResolved(resolved),
	})
	require.NoError(t, err)
	return builder
}

func TestSyntheticProviderConformance(t *testing.T) {
	t.Parallel()
	var requests []map[string]interface{}
	var turn atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/chat/completions", r.URL.Path)
		require.Equal(t, "Bearer synthetic-key", r.Header.Get("Authorization"))
		var body map[string]interface{}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		requests = append(requests, body)
		content := "Hello."
		if turn.Add(1) == 2 {
			content = "```\n[tool_call]{\"tool\":\"done\",\"parameters\":{}}[/tool_call]\n```"
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []interface{}{map[string]interface{}{"message": map[string]interface{}{"role": "assistant", "content": content}}},
			"usage":   map[string]interface{}{"prompt_tokens": 11, "completion_tokens": 2},
		})
	}))
	defer server.Close()
	resolved := &ResolvedModel{}
	builder := syntheticBuilder(t, server.URL, resolved)

	first := builder.Build(core.Result{State: "Composing", Output: "hi"}).Execute()
	second := builder.Build(core.Result{State: "Composing", Output: "finish"}).Execute()

	require.Equal(t, core.LLMResponded, first.Signal)
	require.Equal(t, "Hello.", first.Output)
	require.Equal(t, 11, first.Cost.TokensIn)
	require.Equal(t, 2, first.Cost.TokensOut)
	require.Equal(t, "openai", builder.ProviderName)
	messages := requests[1]["messages"].([]interface{})
	require.Len(t, messages, 4, "system, both user turns, and the first reply travel as an array")
	require.Equal(t, "gpt-synthetic", requests[1]["model"])
	require.Equal(t, `{"tool":"done","parameters":{}}`, resolved.Parser.ExtractToolCall(second.Output),
		"the library's own parser profile, matched by the gpt- prefix, strips the fence and reads the envelope")
}

func TestSyntheticProviderFailuresUseTheSharedTaxonomy(t *testing.T) {
	t.Parallel()
	for status, signal := range map[int]string{401: "ProviderUnauthorized", 429: "ProviderThrottled", 503: "ProviderUnavailable"} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
		}))
		builder := syntheticBuilder(t, server.URL, &ResolvedModel{})

		result := builder.Build(core.Result{State: "Composing", Output: "hi"}).Execute()

		require.Equal(t, core.CommandError, result.Signal)
		require.ErrorContains(t, result.Err, signal)
		server.Close()
	}
}
