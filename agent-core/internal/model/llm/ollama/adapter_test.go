// Copyright (c) 2026 Nokia. All rights reserved.

package ollama

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/model/llm"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/observability/tracing"
)

// ---------------------------------------------------------------------------
// test helpers
// ---------------------------------------------------------------------------

func tagsHandler(models []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			http.NotFound(w, r)
			return
		}
		entries := make([]modelEntry, len(models))
		for i, m := range models {
			entries[i] = modelEntry{Name: m}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tagsResp{Models: entries})
	}
}

func chatAPIHandler(content string, promptEval, eval int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(tagsResp{Models: []modelEntry{{Name: "llama3:latest"}}})
			return
		}
		if r.URL.Path != "/api/chat" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		resp := chatResp{
			Message:         msgDTO{Role: "assistant", Content: content},
			EvalCount:       eval,
			PromptEvalCount: promptEval,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// ---------------------------------------------------------------------------
// recording tracer (for tracing assertions)
// ---------------------------------------------------------------------------

type recordedEvent struct {
	Name  string
	Attrs map[string]interface{}
}

type recordedSpan struct {
	Name      string
	Attrs     map[string]interface{}
	SetAttrs  map[string]interface{}
	HasError  bool
	Completed bool
}

type recordingTracer struct {
	Events []recordedEvent
	Spans  []recordedSpan
	cur    int // index of current span in Spans
}

func newRecordingTracer() *recordingTracer { return &recordingTracer{cur: -1} }

func (r *recordingTracer) Push(name string, attrs ...attribute.KeyValue) (tracing.Tracer, func()) {
	m := make(map[string]interface{})
	for _, a := range attrs {
		m[string(a.Key)] = attrValue(a.Value)
	}
	idx := len(r.Spans)
	r.Spans = append(r.Spans, recordedSpan{Name: name, Attrs: m, SetAttrs: make(map[string]interface{})})
	child := &recordingTracer{Events: r.Events, Spans: r.Spans, cur: idx}
	return child, func() { r.Spans[idx].Completed = true }
}

func (r *recordingTracer) Event(name string, attrs ...attribute.KeyValue) {
	m := make(map[string]interface{})
	for _, a := range attrs {
		m[string(a.Key)] = attrValue(a.Value)
	}
	r.Events = append(r.Events, recordedEvent{Name: name, Attrs: m})
}

func (r *recordingTracer) SetAttributes(attrs ...attribute.KeyValue) {
	if r.cur >= 0 && r.cur < len(r.Spans) {
		for _, a := range attrs {
			r.Spans[r.cur].SetAttrs[string(a.Key)] = attrValue(a.Value)
		}
	}
}

func (r *recordingTracer) RecordError(_ error) {
	if r.cur >= 0 && r.cur < len(r.Spans) {
		r.Spans[r.cur].HasError = true
	}
}

func (r *recordingTracer) Context() context.Context { return context.Background() }

func attrValue(v attribute.Value) interface{} {
	switch v.Type() {
	case attribute.STRING:
		return v.AsString()
	case attribute.INT64:
		return v.AsInt64()
	case attribute.BOOL:
		return v.AsBool()
	default:
		return v.String()
	}
}

func (r *recordingTracer) findEvent(name string) *recordedEvent {
	for i := range r.Events {
		if r.Events[i].Name == name {
			return &r.Events[i]
		}
	}
	return nil
}

var _ tracing.Tracer = (*recordingTracer)(nil)

// ---------------------------------------------------------------------------
// NewAdapter / model check
// ---------------------------------------------------------------------------

func TestNewAdapter_ModelFound(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(tagsHandler([]string{"llama3:latest", "mistral:7b"}))
	defer srv.Close()

	a, err := NewAdapter(srv.URL, "llama3", WithHTTPClient(srv.Client()))
	require.NoError(t, err)
	require.NotNil(t, a)
	require.Equal(t, "llama3", a.Model())
}

func TestNewAdapter_ModelFoundExactTag(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(tagsHandler([]string{"llama3:latest", "llama3:8b"}))
	defer srv.Close()

	a, err := NewAdapter(srv.URL, "llama3:8b", WithHTTPClient(srv.Client()))
	require.NoError(t, err)
	require.NotNil(t, a)
}

func TestNewAdapter_ModelNotFound(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(tagsHandler([]string{"mistral:7b"}))
	defer srv.Close()

	_, err := NewAdapter(srv.URL, "llama3", WithHTTPClient(srv.Client()))
	require.Error(t, err)
	require.Contains(t, err.Error(), `model "llama3" is not available locally`)
	require.Contains(t, err.Error(), "ollama pull llama3")
}

func TestNewAdapter_CaseInsensitiveMatch(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(tagsHandler([]string{"Llama3:Latest"}))
	defer srv.Close()

	a, err := NewAdapter(srv.URL, "llama3", WithHTTPClient(srv.Client()))
	require.NoError(t, err)
	require.NotNil(t, a)
}

func TestNewAdapter_ConnectionRefused(t *testing.T) {
	t.Parallel()
	_, err := NewAdapter("http://127.0.0.1:1", "llama3", WithHTTPClient(&http.Client{}))
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to connect to Ollama")
}

func TestNewAdapter_BadHTTPStatus(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := NewAdapter(srv.URL, "llama3", WithHTTPClient(srv.Client()))
	require.Error(t, err)
	require.Contains(t, err.Error(), "status 500")
}

func TestNewAdapter_MalformedJSON(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	_, err := NewAdapter(srv.URL, "llama3", WithHTTPClient(srv.Client()))
	require.Error(t, err)
	require.Contains(t, err.Error(), "parse /api/tags response")
}

func TestNewAdapter_EmptyModelList(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(tagsHandler([]string{}))
	defer srv.Close()

	_, err := NewAdapter(srv.URL, "llama3", WithHTTPClient(srv.Client()))
	require.Error(t, err)
	require.Contains(t, err.Error(), "not available locally")
}

func TestNewAdapter_TrailingSlashInURL(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(tagsHandler([]string{"llama3:latest"}))
	defer srv.Close()

	a, err := NewAdapter(srv.URL+"/", "llama3", WithHTTPClient(srv.Client()))
	require.NoError(t, err)
	require.NotNil(t, a)
}

// ---------------------------------------------------------------------------
// matchModel
// ---------------------------------------------------------------------------

func TestMatchModel_NoTag_MatchesAnyTag(t *testing.T) {
	t.Parallel()
	models := []modelEntry{{Name: "llama3:latest"}, {Name: "llama3:8b"}}
	require.True(t, matchModel("llama3", models))
}

func TestMatchModel_WithTag_ExactOnly(t *testing.T) {
	t.Parallel()
	models := []modelEntry{{Name: "llama3:latest"}, {Name: "llama3:8b"}}
	require.True(t, matchModel("llama3:8b", models))
	require.False(t, matchModel("llama3:70b", models))
}

func TestMatchModel_CaseInsensitive(t *testing.T) {
	t.Parallel()
	models := []modelEntry{{Name: "Llama3:Latest"}}
	require.True(t, matchModel("LLAMA3", models))
	require.True(t, matchModel("llama3:latest", models))
}

func TestMatchModel_Empty(t *testing.T) {
	t.Parallel()
	require.False(t, matchModel("llama3", nil))
	require.False(t, matchModel("llama3", []modelEntry{}))
}

// ---------------------------------------------------------------------------
// JSON round-trip
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Adapter.Chat
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// ListModels
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// tracing
// ---------------------------------------------------------------------------

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
