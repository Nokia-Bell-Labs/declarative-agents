// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package client_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	toolrest "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest"
)

// captureTracer accumulates every attribute set on the dispatch span, unlike a
// last-write-wins double: one capture sets its body and its truncation flag in
// separate calls.
type captureTracer struct{ attributes []attribute.KeyValue }

func (c *captureTracer) Push(string, ...attribute.KeyValue) (tracing.Tracer, func()) {
	return c, func() {}
}

func (*captureTracer) Event(string, ...attribute.KeyValue) {}

func (c *captureTracer) SetAttributes(attrs ...attribute.KeyValue) {
	c.attributes = append(c.attributes, attrs...)
}

func (*captureTracer) RecordError(error) {}

func (*captureTracer) Context() context.Context { return context.Background() }

func (c *captureTracer) value(key string) (attribute.Value, bool) {
	for _, attr := range c.attributes {
		if string(attr.Key) == key {
			return attr.Value, true
		}
	}
	return attribute.Value{}, false
}

func (c *captureTracer) body(t *testing.T, key string) map[string]interface{} {
	t.Helper()
	value, ok := c.value(key)
	require.Truef(t, ok, "expected attribute %s on the dispatch span", key)
	decoded := map[string]interface{}{}
	require.NoError(t, json.Unmarshal([]byte(value.AsString()), &decoded))
	return decoded
}

// captureCommand builds a client command at the given capture setting and
// injects a recording tracer the way core.Dispatch injects the real one.
func captureCommand(
	t *testing.T,
	def Definition,
	init string,
	operation string,
	input map[string]interface{},
	capture bool,
) (core.Command, *captureTracer) {
	t.Helper()
	collection := toolrest.NewCollection()
	require.NoError(t, collection.Add(def))
	resolved, err := collection.ResolveClientOperation(toolrest.ClientToolConfig{
		RestRef: "github", Resource: "issue", Operation: operation,
	})
	require.NoError(t, err)
	params, err := json.Marshal(map[string]interface{}{"tool": init, "parameters": input})
	require.NoError(t, err)
	cmd := ClientBuilder{
		ToolName: init, Init: init, Operation: resolved, CaptureContent: capture,
	}.Build(core.Result{Output: string(params)})
	tracer := &captureTracer{}
	aware, ok := cmd.(core.TracerAware)
	require.True(t, ok, "client command must accept the dispatch tracer")
	aware.SetTracer(tracer)
	return cmd, tracer
}

// secretBodyDefinition gives the set operation a request body carrying a field
// the operation already declares redacted, so capture has something to hide on
// both sides of the call.
func secretBodyDefinition(t *testing.T, baseURL string) Definition {
	t.Helper()
	client := issueClient()
	op := client.Resources["issue"].Operations["set"]
	op.Body = map[string]interface{}{
		"title":  "{{ params.title }}",
		"secret": "request-secret",
	}
	client.Resources["issue"].Operations["set"] = op
	return clientDefinition(t, baseURL, client)
}

func TestRESTCaptureRecordsBothBodiesAtFull(t *testing.T) {
	t.Parallel()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"title":"ok","id":"7"}`))
	}))
	defer upstream.Close()

	cmd, tracer := captureCommand(
		t, clientDefinition(t, upstream.URL, issueClient()),
		InitClientSet, "set", params("1", "new title"), true,
	)
	require.Equal(t, core.Signal("RESTResourceWritten"), cmd.Execute().Signal)

	require.Equal(t, "new title", tracer.body(t, "http.request.body")["title"])
	require.Equal(t, "ok", tracer.body(t, "http.response.body")["title"])
}

func TestRESTCaptureRecordsNothingBelowFull(t *testing.T) {
	t.Parallel()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"title":"ok"}`))
	}))
	defer upstream.Close()

	cmd, tracer := captureCommand(
		t, clientDefinition(t, upstream.URL, issueClient()),
		InitClientSet, "set", params("1", "new title"), false,
	)
	require.Equal(t, core.Signal("RESTResourceWritten"), cmd.Execute().Signal)

	for _, key := range []string{
		"http.request.body", "http.response.body",
		"http.request.body.truncated", "http.response.body.truncated",
	} {
		_, ok := tracer.value(key)
		require.Falsef(t, ok, "capture off and delta must not set %s", key)
	}
}

func TestRESTCaptureAppliesDeclaredRedaction(t *testing.T) {
	t.Parallel()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"title":"ok","secret":"response-secret"}`))
	}))
	defer upstream.Close()

	cmd, tracer := captureCommand(
		t, secretBodyDefinition(t, upstream.URL),
		InitClientSet, "set", params("1", "new title"), true,
	)
	require.Equal(t, core.Signal("RESTResourceWritten"), cmd.Execute().Signal)

	// Assert the redacted marker, not merely absence: a missing key would
	// satisfy a NotEqual while proving nothing about redaction running.
	request := tracer.body(t, "http.request.body")
	require.Equal(t, "new title", request["title"])
	require.Equal(t, "[REDACTED]", request["secret"])

	response := tracer.body(t, "http.response.body")
	require.Equal(t, "ok", response["title"])
	require.Equal(t, "[REDACTED]", response["secret"])

	for _, attr := range tracer.attributes {
		require.NotContains(t, attr.Value.AsString(), "request-secret")
		require.NotContains(t, attr.Value.AsString(), "response-secret")
	}
}

func TestRESTCaptureSkipsNonJSONResponseBody(t *testing.T) {
	t.Parallel()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("plain text, not an object"))
	}))
	defer upstream.Close()

	cmd, tracer := captureCommand(
		t, clientDefinition(t, upstream.URL, issueClient()),
		InitClientSet, "set", params("1", "new title"), true,
	)
	cmd.Execute()

	_, ok := tracer.value("http.response.body")
	require.False(t, ok, "a body that is not a JSON object cannot be redacted, so it is not captured")
	require.Equal(t, "new title", tracer.body(t, "http.request.body")["title"])
}

func TestRESTCaptureTruncatesOversizedBody(t *testing.T) {
	t.Parallel()
	oversized, err := json.Marshal(map[string]interface{}{
		"title": strings.Repeat("x", 20000),
	})
	require.NoError(t, err)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(oversized)
	}))
	defer upstream.Close()

	cmd, tracer := captureCommand(
		t, clientDefinition(t, upstream.URL, issueClient()),
		InitClientSet, "set", params("1", "new title"), true,
	)
	require.Equal(t, core.Signal("RESTResourceWritten"), cmd.Execute().Signal)

	truncated, ok := tracer.value("http.response.body.truncated")
	require.True(t, ok, "an oversized body must be flagged truncated")
	require.True(t, truncated.AsBool())

	recorded, ok := tracer.value("http.response.body")
	require.True(t, ok)
	require.Len(t, recorded.AsString(), 16384)
}
