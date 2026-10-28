// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	modelllm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/credentials"
)

// The Anthropic and Vertex AI libraries (srd058 R1.3, GH-2805) bind the
// OpenAI-compatible endpoints both platforms serve, so the srd058 R5.3 checks
// the synthetic library established run against the shipped files too: the
// unchanged invoke_llm word, the bearer credential resolved at call time, the
// library's own fence-stripping parser matched by model prefix, and the R1.4
// failure taxonomy.

type shippedOpenAIShaped struct {
	library    string
	path       string
	credential string
	model      string
	provider   string
}

var shippedOpenAIShapedLibraries = []shippedOpenAIShaped{
	{library: "anthropic", path: "/v1/chat/completions", credential: "ANTHROPIC_API_KEY",
		model: "claude-opus-5-5", provider: "anthropic"},
	{library: "vertex", path: "/chat/completions", credential: "VERTEX_ACCESS_TOKEN",
		model: "google/gemini-2.5-flash", provider: "gcp.vertex_ai"},
}

func shippedOpenAIShapedBuilder(t *testing.T, entry shippedOpenAIShaped, url string, resolved *ResolvedModel) *InvokeLLMBuilder {
	t.Helper()
	builder, err := NewInvokeLLMBuilder(catalog.ToolDef{Name: "invoke_llm", Config: map[string]interface{}{
		"dialect": shippedDialect(t, entry.library), "provider_url": url,
		"model": entry.model, "manifest_state": "Composing", "system_prompt": "Answer briefly.",
	}}, InvokeLLMFactoryDeps{
		History:  modelllm.NewConversation(nil, "", modelllm.ChatOptions{}),
		Registry: core.NewRegistry(), Tracer: tracing.NoopTracer{}, Ctx: context.Background(),
		Credentials: credentials.Static{entry.credential: "shipped-key"},
		OnResolved:  applyResolved(resolved),
	})
	require.NoError(t, err)
	return builder
}

func TestShippedOpenAIShapedConformance(t *testing.T) {
	t.Parallel()
	for _, entry := range shippedOpenAIShapedLibraries {
		t.Run(entry.library, func(t *testing.T) {
			t.Parallel()
			var requests []map[string]interface{}
			var turn atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, entry.path, r.URL.Path)
				require.Equal(t, "Bearer shipped-key", r.Header.Get("Authorization"))
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
			builder := shippedOpenAIShapedBuilder(t, entry, server.URL, resolved)

			first := builder.Build(core.Result{State: "Composing", Output: "hi"}).Execute()
			second := builder.Build(core.Result{State: "Composing", Output: "finish"}).Execute()

			require.Equal(t, core.LLMResponded, first.Signal)
			require.Equal(t, "Hello.", first.Output)
			require.Equal(t, 11, first.Cost.TokensIn)
			require.Equal(t, 2, first.Cost.TokensOut)
			require.Equal(t, entry.provider, builder.ProviderName)
			messages := requests[1]["messages"].([]interface{})
			require.Len(t, messages, 4, "system, both user turns, and the first reply travel as an array")
			require.Equal(t, entry.model, requests[1]["model"])
			require.Equal(t, `{"tool":"done","parameters":{}}`, resolved.Parser.ExtractToolCall(second.Output),
				"the library's own parser profile, matched by the model prefix, strips the fence and reads the envelope")
		})
	}
}

func TestShippedOpenAIShapedFailuresUseTheSharedTaxonomy(t *testing.T) {
	t.Parallel()
	for _, entry := range shippedOpenAIShapedLibraries {
		t.Run(entry.library, func(t *testing.T) {
			t.Parallel()
			for status, signal := range map[int]string{401: "ProviderUnauthorized", 429: "ProviderThrottled", 503: "ProviderUnavailable"} {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(status)
				}))
				builder := shippedOpenAIShapedBuilder(t, entry, server.URL, &ResolvedModel{})

				result := builder.Build(core.Result{State: "Composing", Output: "hi"}).Execute()

				require.Equal(t, core.CommandError, result.Signal)
				require.ErrorContains(t, result.Err, signal)
				server.Close()
			}
		})
	}
}
