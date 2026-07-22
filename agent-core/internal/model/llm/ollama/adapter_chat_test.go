// Copyright (c) 2026 Nokia. All rights reserved.

package ollama

import (
	"context"
	"encoding/json"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestChatReq_JSONRoundTrip(t *testing.T) {
	t.Parallel()
	req := chatReq{
		Model: "llama3",
		Messages: []msgDTO{
			{Role: "system", Content: "You are a helpful assistant."},
			{Role: "user", Content: "Hello"},
		},
		Stream:  false,
		Options: chatOpts{Temperature: 0, Seed: 42, NumCtx: 8192},
	}

	data, err := json.Marshal(req)
	require.NoError(t, err)

	var decoded chatReq
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.Equal(t, "llama3", decoded.Model)
	require.False(t, decoded.Stream)
	require.Len(t, decoded.Messages, 2)
	require.Equal(t, float64(0), decoded.Options.Temperature)
	require.Equal(t, 42, decoded.Options.Seed)
	require.Equal(t, 8192, decoded.Options.NumCtx)
}

func TestChatReq_NumCtxOmittedWhenZero(t *testing.T) {
	t.Parallel()
	req := chatReq{
		Model:   "llama3",
		Stream:  false,
		Options: chatOpts{Temperature: 0, Seed: 42},
	}

	data, err := json.Marshal(req)
	require.NoError(t, err)
	require.NotContains(t, string(data), "num_ctx")
}

func TestChatResp_TokenExtraction(t *testing.T) {
	t.Parallel()
	raw := `{"message":{"role":"assistant","content":"Hello!"},"eval_count":15,"prompt_eval_count":100}`

	var resp chatResp
	require.NoError(t, json.Unmarshal([]byte(raw), &resp))
	require.Equal(t, "Hello!", resp.Message.Content)
	require.Equal(t, "assistant", resp.Message.Role)
	require.Equal(t, 15, resp.EvalCount)
	require.Equal(t, 100, resp.PromptEvalCount)
}

func TestChatResp_MissingTokenCounts(t *testing.T) {
	t.Parallel()
	raw := `{"message":{"role":"assistant","content":"Hi"}}`

	var resp chatResp
	require.NoError(t, json.Unmarshal([]byte(raw), &resp))
	require.Equal(t, "Hi", resp.Message.Content)
	require.Zero(t, resp.EvalCount)
	require.Zero(t, resp.PromptEvalCount)
}

func TestChat_Success(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(chatAPIHandler("hello back", 100, 15))
	defer srv.Close()

	a, err := NewAdapter(srv.URL, "llama3", WithHTTPClient(srv.Client()))
	require.NoError(t, err)

	msgs := []llm.Message{
		{Role: llm.System, Content: "be helpful"},
		{Role: llm.User, Content: "hello"},
	}
	opts := llm.ChatOptions{Model: "llama3", Temperature: 0, Seed: 42}
	resp, err := a.Chat(context.Background(), msgs, opts)
	require.NoError(t, err)
	require.Equal(t, "hello back", resp.Content)
	require.Equal(t, 100, resp.TokensIn)
	require.Equal(t, 15, resp.TokensOut)
}

func TestChat_NumCtxPassedToOllama(t *testing.T) {
	t.Parallel()
	var captured chatReq
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(tagsResp{Models: []modelEntry{{Name: "llama3:latest"}}})
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&captured)
		resp := chatResp{
			Message:         msgDTO{Role: "assistant", Content: "ok"},
			EvalCount:       5,
			PromptEvalCount: 10,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	a, err := NewAdapter(srv.URL, "llama3", WithHTTPClient(srv.Client()))
	require.NoError(t, err)

	msgs := []llm.Message{{Role: llm.User, Content: "hi"}}
	opts := llm.ChatOptions{Model: "llama3", NumCtx: 16384}
	_, err = a.Chat(context.Background(), msgs, opts)
	require.NoError(t, err)
	require.Equal(t, 16384, captured.Options.NumCtx)
}

func TestChat_NumCtxZeroOmitted(t *testing.T) {
	t.Parallel()
	var rawBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(tagsResp{Models: []modelEntry{{Name: "llama3:latest"}}})
			return
		}
		rawBody, _ = io.ReadAll(r.Body)
		resp := chatResp{
			Message:         msgDTO{Role: "assistant", Content: "ok"},
			EvalCount:       5,
			PromptEvalCount: 10,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	a, err := NewAdapter(srv.URL, "llama3", WithHTTPClient(srv.Client()))
	require.NoError(t, err)

	msgs := []llm.Message{{Role: llm.User, Content: "hi"}}
	_, err = a.Chat(context.Background(), msgs, llm.ChatOptions{Model: "llama3"})
	require.NoError(t, err)
	require.NotContains(t, string(rawBody), "num_ctx",
		"num_ctx should be omitted when zero so Ollama uses model default")
}

func TestChat_HTTPError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(tagsResp{Models: []modelEntry{{Name: "llama3:latest"}}})
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("overloaded"))
	}))
	defer srv.Close()

	a, err := NewAdapter(srv.URL, "llama3", WithHTTPClient(srv.Client()))
	require.NoError(t, err)

	_, err = a.Chat(context.Background(), []llm.Message{{Role: llm.User, Content: "hi"}}, llm.ChatOptions{Model: "llama3"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "status 503")
}

func TestChat_MalformedResponse(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(tagsResp{Models: []modelEntry{{Name: "llama3:latest"}}})
			return
		}
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	a, err := NewAdapter(srv.URL, "llama3", WithHTTPClient(srv.Client()))
	require.NoError(t, err)

	_, err = a.Chat(context.Background(), []llm.Message{{Role: llm.User, Content: "hi"}}, llm.ChatOptions{Model: "llama3"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "parse chat response")
}

func TestListModels_Success(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := detailedTagsResp{Models: []detailedModelEntry{
			{Name: "llama3:latest", Size: 4_000_000_000, Digest: "abc123"},
			{Name: "mistral:7b", Size: 7_000_000_000, Digest: "def456"},
		}}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	a := &Adapter{baseURL: srv.URL, client: srv.Client()}
	models, err := a.ListModels(context.Background())
	require.NoError(t, err)
	require.Len(t, models, 2)
	require.Equal(t, "llama3:latest", models[0].Name)
	require.Equal(t, "ollama", models[0].Provider)
	require.Equal(t, "abc123", models[0].Details["digest"])
}
