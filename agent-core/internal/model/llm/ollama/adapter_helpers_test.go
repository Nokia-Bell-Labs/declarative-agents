// Copyright (c) 2026 Nokia. All rights reserved.

package ollama

import (
	"context"
	"encoding/json"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"net/http"
	"os"
	"testing"
)

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

func assertFileContains(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(data), want)
}
