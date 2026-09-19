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
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	modelllm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/credentials"
)

// The compiled Ollama and Cohere adapters retired after a parity run showed
// the shipped dialects reproduce their requests, histories, signals, and spans
// (srd058 R5.2, GH-2240). testdata/parity holds what the adapters sent and
// answered for a three-turn conversation; these tests hold the dialect path,
// reached by dialect and by a legacy provider value alike, to that capture.

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

type capturedTurn struct {
	Request   recordedRequest `json:"request"`
	Signal    string          `json:"signal"`
	Output    string          `json:"output"`
	TokensIn  int             `json:"tokens_in"`
	TokensOut int             `json:"tokens_out"`
	Reply     string          `json:"reply"`
}

type capture struct {
	Provider string         `json:"provider"`
	Model    string         `json:"model"`
	Turns    []capturedTurn `json:"turns"`
}

func loadCapture(t *testing.T, provider string) capture {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "parity", provider+".json"))
	require.NoError(t, err)
	var captured capture
	require.NoError(t, json.Unmarshal(data, &captured))
	require.Len(t, captured.Turns, 3)
	return captured
}

func TestProviderDialectParity(t *testing.T) {
	t.Setenv("COHERE_API_KEY", "parity-key")
	for _, provider := range []string{"ollama", "cohere"} {
		t.Run(provider, func(t *testing.T) {
			captured := loadCapture(t, provider)
			replies := make([]string, len(captured.Turns))
			for index, turn := range captured.Turns {
				replies[index] = turn.Reply
			}
			server := &replayServer{replies: replies}
			httpServer := httptest.NewServer(server)
			defer httpServer.Close()

			byDialect := runConversation(t, server, httpServer.URL,
				map[string]interface{}{"dialect": shippedDialect(t, provider), "model": captured.Model})
			byProvider := runConversation(t, server, httpServer.URL,
				map[string]interface{}{"provider": provider, "model": captured.Model})

			for index, turn := range captured.Turns {
				require.Equal(t, turn.Request, byDialect.requests[index],
					"turn %d: the dialect sends what the adapter sent", index+1)
				result := byDialect.results[index]
				require.Equal(t, turn.Signal, string(result.Signal), "turn %d signal", index+1)
				require.Equal(t, turn.TokensIn, result.Cost.TokensIn)
				require.Equal(t, turn.TokensOut, result.Cost.TokensOut)
				if result.Signal == core.LLMResponded {
					require.Equal(t, turn.Output, result.Output)
				}
			}
			require.Equal(t, byDialect.requests, byProvider.requests, "a legacy provider value takes the same path")
			require.Equal(t, byDialect.history, byProvider.history)
			require.Equal(t, byDialect.spans, byProvider.spans)
		})
	}
}

func TestProviderDialectFailureKeepsCommandError(t *testing.T) {
	t.Setenv("COHERE_API_KEY", "parity-key")
	for _, provider := range []string{"ollama", "cohere"} {
		t.Run(provider, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"message":"overloaded"}`))
			}))
			defer server.Close()
			recorder := tracing.NewRecordingTracer()
			builder, err := NewInvokeLLMBuilder(catalog.ToolDef{Name: "invoke_llm", Config: map[string]interface{}{
				"provider": provider, "provider_url": server.URL, "manifest_state": "Composing", "model": "m",
			}}, InvokeLLMFactoryDeps{
				History:  modelllm.NewConversation(nil, "", modelllm.ChatOptions{}),
				Registry: core.NewRegistry(), Tracer: recorder, Ctx: context.Background(),
			})
			require.NoError(t, err)

			result := builder.Build(core.Result{State: "Composing", Output: "q"}).Execute()

			require.Equal(t, core.CommandError, result.Signal)
			require.ErrorContains(t, result.Err, "status 503 (ProviderUnavailable)")
			require.Equal(t, "503", recorder.Spans[0].SetAttrs["error.type"], "the adapters' error type")
		})
	}
}
