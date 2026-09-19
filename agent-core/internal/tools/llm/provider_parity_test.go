// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	modelllm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/credentials"
)

// The capture-fixture parity run srd058 R5.2 requires before the compiled
// adapters retire: the same multi-turn conversation goes through a provider's
// adapter and through its shipped chat dialect against one recorded server,
// and the requests, results, histories, and spans must match (AC2).

type recordedRequest struct {
	Path          string
	Authorization string
	Body          map[string]interface{}
}

// replayServer answers each turn with the next recorded reply and keeps every
// request it received.
type replayServer struct {
	mu       sync.Mutex
	replies  []string
	turn     int
	requests []recordedRequest
}

func (s *replayServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var body map[string]interface{}
	_ = json.NewDecoder(r.Body).Decode(&body)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, recordedRequest{
		Path: r.URL.Path, Authorization: r.Header.Get("Authorization"), Body: body,
	})
	_, _ = w.Write([]byte(s.replies[s.turn%len(s.replies)]))
	s.turn++
}

type parityRun struct {
	requests []recordedRequest
	results  []core.Result
	history  []modelllm.Message
	spans    []tracing.RecordedSpan
}

// runConversation drives three turns through one invoke_llm configuration.
// Both sides of a comparison share one server, so the span's server address
// and port are the same.
func runConversation(t *testing.T, server *replayServer, url string, config map[string]interface{}) parityRun {
	t.Helper()
	server.mu.Lock()
	server.turn, server.requests = 0, nil
	server.mu.Unlock()
	def := catalog.ToolDef{Name: "invoke_llm", Config: map[string]interface{}{
		"provider_url": url, "manifest_state": "Composing",
		"system_prompt": "You answer briefly.", "num_ctx": 8192,
	}}
	for key, value := range config {
		def.Config[key] = value
	}
	recorder := tracing.NewRecordingTracer()
	builder, err := NewInvokeLLMBuilder(def, InvokeLLMFactoryDeps{
		History:  modelllm.NewConversation(nil, "", modelllm.ChatOptions{}),
		Registry: core.NewRegistry(), Tracer: recorder, Ctx: context.Background(),
		Credentials: credentials.Environment{},
	})
	require.NoError(t, err)

	var run parityRun
	for _, prompt := range []string{"first question", "second question", "third question"} {
		result := builder.Build(core.Result{State: "Composing", Output: prompt}).Execute()
		result.Cost.Duration = 0
		run.results = append(run.results, result)
	}
	run.requests = server.requests
	run.history = builder.History.Snapshot()
	run.spans = recorder.Spans
	return run
}

func shippedDialect(t *testing.T, provider string) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "..", "..", "tools", "providers", provider, "chat-dialect.yaml"))
	require.NoError(t, err)
	return path
}

func requireParity(t *testing.T, adapter, dialect parityRun) {
	t.Helper()
	require.Len(t, dialect.requests, len(adapter.requests))
	for index := range adapter.requests {
		require.Equal(t, adapter.requests[index], dialect.requests[index],
			"turn %d request: same path, credential, and JSON document", index+1)
	}
	require.Equal(t, comparable(adapter.results), comparable(dialect.results), "same signals, outputs, usage, and receipts")
	require.Equal(t, adapter.history, dialect.history, "same conversation history")
	require.Equal(t, adapter.spans, dialect.spans, "same inference spans and attributes")
}

// comparable drops a failed turn's error text, which names the failure in
// each side's own words; the signal, usage, and span error type still compare.
func comparable(results []core.Result) []core.Result {
	out := make([]core.Result, len(results))
	for index, result := range results {
		if result.Signal == core.CommandError {
			result.Err, result.Output = nil, ""
		}
		out[index] = result
	}
	return out
}

func TestProviderDialectParity(t *testing.T) {
	t.Setenv("COHERE_API_KEY", "parity-key")
	cases := map[string]struct {
		model   string
		replies []string
	}{
		"ollama": {model: "qwen3:8b", replies: []string{
			`{"message":{"role":"assistant","content":"one"},"prompt_eval_count":21,"eval_count":3}`,
			`{"message":{"role":"assistant","content":"two"},"prompt_eval_count":34,"eval_count":4}`,
			`{"message":{"role":"assistant","content":""},"prompt_eval_count":40,"eval_count":0}`,
		}},
		"cohere": {model: "command-a-03-2025", replies: []string{
			`{"message":{"content":[{"type":"text","text":"one"}]},"usage":{"tokens":{"input_tokens":21,"output_tokens":3}}}`,
			`{"message":{"content":[{"type":"thinking","thinking":"hm"},{"type":"text","text":"tw"},{"type":"text","text":"o"}]},"usage":{"tokens":{"input_tokens":34,"output_tokens":4}}}`,
			`{"message":{"content":[]},"usage":{"tokens":{"input_tokens":40,"output_tokens":0}}}`,
		}},
	}
	for provider, tc := range cases {
		t.Run(provider, func(t *testing.T) {
			server := &replayServer{replies: tc.replies}
			httpServer := httptest.NewServer(server)
			defer httpServer.Close()
			adapter := runConversation(t, server, httpServer.URL, map[string]interface{}{"provider": provider, "model": tc.model})
			dialect := runConversation(t, server, httpServer.URL, map[string]interface{}{"dialect": shippedDialect(t, provider), "model": tc.model})

			require.Len(t, adapter.requests, 3)
			messages, ok := dialect.requests[2].Body["messages"].([]interface{})
			require.True(t, ok, "the conversation is placed as an array")
			require.Len(t, messages, 6, "system, three user turns, two assistant replies")
			requireParity(t, adapter, dialect)
		})
	}
}

// TestProviderDialectParityOnFailure holds the failure path to the same
// signal and span error type; only the message gains the dialect's failure
// signal name.
func TestProviderDialectParityOnFailure(t *testing.T) {
	t.Setenv("COHERE_API_KEY", "parity-key")
	for _, provider := range []string{"ollama", "cohere"} {
		t.Run(provider, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"message":"overloaded"}`))
			}))
			defer server.Close()
			run := func(config map[string]interface{}) (core.Result, []tracing.RecordedSpan) {
				recorder := tracing.NewRecordingTracer()
				def := catalog.ToolDef{Name: "invoke_llm", Config: map[string]interface{}{
					"provider_url": server.URL, "manifest_state": "Composing", "model": "m",
				}}
				for key, value := range config {
					def.Config[key] = value
				}
				builder, err := NewInvokeLLMBuilder(def, InvokeLLMFactoryDeps{
					History:  modelllm.NewConversation(nil, "", modelllm.ChatOptions{}),
					Registry: core.NewRegistry(), Tracer: recorder, Ctx: context.Background(),
				})
				require.NoError(t, err)
				return builder.Build(core.Result{State: "Composing", Output: "q"}).Execute(), recorder.Spans
			}

			adapterResult, adapterSpans := run(map[string]interface{}{"provider": provider})
			dialectResult, dialectSpans := run(map[string]interface{}{"dialect": shippedDialect(t, provider)})

			require.Equal(t, adapterResult.Signal, dialectResult.Signal)
			require.Equal(t, core.CommandError, dialectResult.Signal)
			require.ErrorContains(t, dialectResult.Err, "status 503 (ProviderUnavailable)")
			require.Equal(t, adapterSpans[0].SetAttrs["error.type"], dialectSpans[0].SetAttrs["error.type"])
			require.Equal(t, adapterSpans[0].Attrs, dialectSpans[0].Attrs)
		})
	}
}
