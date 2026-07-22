// Copyright (c) 2026 Nokia. All rights reserved.

package ollama

import (
	"context"
	"encoding/json"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewAdapter_TracesCheckModelLifecycle(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(tagsHandler([]string{"llama3:latest", "mistral:7b"}))
	defer srv.Close()

	tr := newRecordingTracer()
	a, err := NewAdapter(srv.URL, "llama3", WithHTTPClient(srv.Client()), WithTracer(tr))
	require.NoError(t, err)
	require.NotNil(t, a)

	start := tr.findEvent("check_model.start")
	require.NotNil(t, start)
	require.Equal(t, "llama3", start.Attrs["llm.model"])

	done := tr.findEvent("check_model.done")
	require.NotNil(t, done)
	require.Equal(t, int64(2), done.Attrs["model_count"])
	require.Equal(t, true, done.Attrs["match"])
}

func TestNewAdapter_TracesCheckModelNotFound(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(tagsHandler([]string{"mistral:7b"}))
	defer srv.Close()

	tr := newRecordingTracer()
	_, err := NewAdapter(srv.URL, "llama3", WithHTTPClient(srv.Client()), WithTracer(tr))
	require.Error(t, err)

	done := tr.findEvent("check_model.done")
	require.NotNil(t, done)
	require.Equal(t, false, done.Attrs["match"])
}

func TestNewAdapter_TracesCheckModelConnectionError(t *testing.T) {
	t.Parallel()
	tr := newRecordingTracer()
	_, err := NewAdapter("http://127.0.0.1:1", "llama3", WithHTTPClient(&http.Client{}), WithTracer(tr))
	require.Error(t, err)

	errEvt := tr.findEvent("check_model.error")
	require.NotNil(t, errEvt)
	require.Contains(t, errEvt.Attrs["error"], "connect")
}

func TestNewAdapter_TracesCheckModelBadStatus(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	tr := newRecordingTracer()
	_, err := NewAdapter(srv.URL, "llama3", WithHTTPClient(srv.Client()), WithTracer(tr))
	require.Error(t, err)

	errEvt := tr.findEvent("check_model.error")
	require.NotNil(t, errEvt)
	require.Equal(t, int64(500), errEvt.Attrs["http.status_code"])
}

func TestChat_EmitsSemconvInferenceSpan(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(chatAPIHandler("hello", 100, 15))
	defer srv.Close()

	tr := newRecordingTracer()
	a, err := NewAdapter(srv.URL, "llama3", WithHTTPClient(srv.Client()), WithTracer(tr))
	require.NoError(t, err)

	msgs := []llm.Message{{Role: llm.User, Content: "hi"}}
	resp, err := a.Chat(context.Background(), msgs, llm.ChatOptions{Model: "llama3"})
	require.NoError(t, err)
	require.Equal(t, "hello", resp.Content)

	require.Len(t, tr.Spans, 1, "Chat should create exactly one span")
	s := tr.Spans[0]

	require.Equal(t, "chat llama3", s.Name)
	require.True(t, s.Completed, "span must be completed")

	require.Equal(t, "chat", s.Attrs["gen_ai.operation.name"])
	require.Equal(t, "ollama", s.Attrs["gen_ai.provider.name"])
	require.Equal(t, "llama3", s.Attrs["gen_ai.request.model"])

	require.Equal(t, int64(100), s.SetAttrs["gen_ai.usage.input_tokens"])
	require.Equal(t, int64(15), s.SetAttrs["gen_ai.usage.output_tokens"])
	require.Equal(t, "llama3", s.SetAttrs["gen_ai.response.model"])
}

func TestChat_SpanRecordsErrorOnHTTPFailure(t *testing.T) {
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

	tr := newRecordingTracer()
	a, err := NewAdapter(srv.URL, "llama3", WithHTTPClient(srv.Client()), WithTracer(tr))
	require.NoError(t, err)

	_, err = a.Chat(context.Background(), []llm.Message{{Role: llm.User, Content: "hi"}}, llm.ChatOptions{Model: "llama3"})
	require.Error(t, err)

	require.Len(t, tr.Spans, 1)
	s := tr.Spans[0]
	require.True(t, s.HasError, "span should record error")
	require.Equal(t, "503", s.SetAttrs["error.type"])
}

func TestChat_NilTracerDoesNotPanic(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(chatAPIHandler("ok", 10, 5))
	defer srv.Close()

	a, err := NewAdapter(srv.URL, "llama3", WithHTTPClient(srv.Client()))
	require.NoError(t, err)

	resp, err := a.Chat(context.Background(), []llm.Message{{Role: llm.User, Content: "hi"}}, llm.ChatOptions{Model: "llama3"})
	require.NoError(t, err)
	require.Equal(t, "ok", resp.Content)
}

func TestNewAdapter_NilTracerNoops(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(tagsHandler([]string{"llama3:latest"}))
	defer srv.Close()

	a, err := NewAdapter(srv.URL, "llama3", WithHTTPClient(srv.Client()))
	require.NoError(t, err)
	require.NotNil(t, a)
}
