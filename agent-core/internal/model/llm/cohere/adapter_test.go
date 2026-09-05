// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package cohere

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
	"github.com/stretchr/testify/require"
)

func staticKey(value string) APIKeyLookup {
	return func() (string, bool) { return value, value != "" }
}

func TestNewAdapter_NoIO(t *testing.T) {
	t.Parallel()
	var lookups atomic.Int32
	adapter, err := NewAdapter("http://127.0.0.1:1/", "command-r7b-12-2024",
		WithAPIKeyLookup(func() (string, bool) {
			lookups.Add(1)
			return "", false
		}),
	)
	require.NoError(t, err)
	require.Equal(t, "http://127.0.0.1:1", adapter.baseURL)
	require.Equal(t, "command-r7b-12-2024", adapter.Model())
	require.Zero(t, lookups.Load())
}

func TestChat_Request(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		require.Equal(t, http.MethodPost, request.Method)
		require.Equal(t, "/v2/chat", request.URL.Path)
		require.Equal(t, "Bearer test-secret", request.Header.Get("Authorization"))
		var body map[string]interface{}
		require.NoError(t, json.NewDecoder(request.Body).Decode(&body))
		require.Equal(t, "command-r7b-12-2024", body["model"])
		require.Equal(t, false, body["stream"])
		require.Equal(t, 0.2, body["temperature"])
		require.Equal(t, float64(42), body["seed"])
		require.NotContains(t, body, "options")
		require.NotContains(t, body, "num_ctx")
		messages := body["messages"].([]interface{})
		require.Equal(t, "system", messages[0].(map[string]interface{})["role"])
		require.Equal(t, "rules", messages[0].(map[string]interface{})["content"])
		require.Equal(t, "user", messages[1].(map[string]interface{})["role"])
		writeResponse(t, w, []contentBlock{{Type: "text", Text: "ok"}}, 11, 3)
	}))
	defer server.Close()

	adapter, err := NewAdapter(server.URL, "command-r7b-12-2024",
		WithHTTPClient(server.Client()), WithAPIKeyLookup(staticKey("test-secret")))
	require.NoError(t, err)
	response, err := adapter.Chat(context.Background(), []llm.Message{
		{Role: llm.System, Content: "rules"},
		{Role: llm.User, Content: "question"},
	}, llm.ChatOptions{Model: "command-r7b-12-2024", Temperature: 0.2, Seed: 42, NumCtx: 8192})
	require.NoError(t, err)
	require.Equal(t, "ok", response.Content)
	require.Equal(t, int32(1), calls.Load())
}

func TestChat_Response(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeResponse(t, w, []contentBlock{
			{Type: "text", Text: `{"names":`},
			{Type: "image", Text: "not-model-text"},
			{Type: "text", Text: `["rag0"]}`},
		}, 27, 8)
	}))
	defer server.Close()
	adapter, err := NewAdapter(server.URL, "command-r7b-12-2024",
		WithHTTPClient(server.Client()), WithAPIKeyLookup(staticKey("key")))
	require.NoError(t, err)

	response, err := adapter.Chat(context.Background(), nil, llm.ChatOptions{Model: "command-r7b-12-2024"})
	require.NoError(t, err)
	require.Equal(t, `{"names":["rag0"]}`, response.Content)
	require.Equal(t, 27, response.TokensIn)
	require.Equal(t, 8, response.TokensOut)
	require.NotContains(t, response.Content, "not-model-text")
}

func TestChat_MissingUsageDefaultsToZero(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"message":{"content":[{"type":"text","text":"ok"}]}}`))
	}))
	defer server.Close()
	adapter, err := NewAdapter(server.URL, "command-r7b-12-2024",
		WithHTTPClient(server.Client()), WithAPIKeyLookup(staticKey("key")))
	require.NoError(t, err)

	response, err := adapter.Chat(context.Background(), nil, llm.ChatOptions{Model: "command-r7b-12-2024"})
	require.NoError(t, err)
	require.Zero(t, response.TokensIn)
	require.Zero(t, response.TokensOut)
}

func TestChat_DefaultLookupUsesEnvironment(t *testing.T) {
	t.Setenv(apiKeyEnvironment, "environment-secret")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		require.Equal(t, "Bearer environment-secret", request.Header.Get("Authorization"))
		writeResponse(t, w, []contentBlock{{Type: "text", Text: "ok"}}, 1, 1)
	}))
	defer server.Close()
	adapter, err := NewAdapter(server.URL, "command-r7b-12-2024", WithHTTPClient(server.Client()))
	require.NoError(t, err)

	_, err = adapter.Chat(context.Background(), nil, llm.ChatOptions{Model: "command-r7b-12-2024"})
	require.NoError(t, err)
}

func TestChat_Errors(t *testing.T) {
	t.Parallel()
	t.Run("missing credential before network", func(t *testing.T) {
		var calls atomic.Int32
		client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			calls.Add(1)
			return nil, errors.New("unexpected")
		})}
		adapter, err := NewAdapter("http://provider.invalid", "command-r7b-12-2024",
			WithHTTPClient(client), WithAPIKeyLookup(staticKey("")))
		require.NoError(t, err)
		_, err = adapter.Chat(context.Background(), nil, llm.ChatOptions{})
		require.ErrorContains(t, err, "COHERE_API_KEY is not resolved")
		require.Zero(t, calls.Load())
	})
	t.Run("request encoding", func(t *testing.T) {
		adapter, err := NewAdapter("http://provider.invalid", "model", WithAPIKeyLookup(staticKey("key")))
		require.NoError(t, err)
		_, err = adapter.Chat(context.Background(), nil, llm.ChatOptions{Temperature: math.NaN()})
		require.ErrorContains(t, err, "marshal Cohere chat request")
	})
	t.Run("request creation", func(t *testing.T) {
		adapter, err := NewAdapter("://bad", "model", WithAPIKeyLookup(staticKey("key")))
		require.NoError(t, err)
		_, err = adapter.Chat(context.Background(), nil, llm.ChatOptions{})
		require.ErrorContains(t, err, "create Cohere chat request")
	})
	t.Run("connection", func(t *testing.T) {
		adapter, err := NewAdapter("http://127.0.0.1:1", "model", WithAPIKeyLookup(staticKey("key")))
		require.NoError(t, err)
		_, err = adapter.Chat(context.Background(), nil, llm.ChatOptions{})
		require.ErrorContains(t, err, "cohere chat request failed")
	})
	t.Run("response read", func(t *testing.T) {
		client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: errorReader{}}, nil
		})}
		adapter, err := NewAdapter("http://provider.invalid", "model",
			WithHTTPClient(client), WithAPIKeyLookup(staticKey("key")))
		require.NoError(t, err)
		_, err = adapter.Chat(context.Background(), nil, llm.ChatOptions{})
		require.ErrorContains(t, err, "read Cohere chat response")
	})
	t.Run("malformed response", func(t *testing.T) {
		err := chatServerError(t, http.StatusOK, "not-json", "key")
		require.ErrorContains(t, err, "parse Cohere chat response")
	})
	t.Run("missing text", func(t *testing.T) {
		err := chatServerError(t, http.StatusOK, `{"message":{"content":[{"type":"image","text":"hidden"}]}}`, "key")
		require.ErrorContains(t, err, "contains no text content")
	})
}

func TestChat_HTTPErrorRedactsCredentialAndDoesNotRetry(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"bad key super-secret"}`))
	}))
	defer server.Close()
	adapter, err := NewAdapter(server.URL, "model",
		WithHTTPClient(server.Client()), WithAPIKeyLookup(staticKey("super-secret")))
	require.NoError(t, err)

	_, err = adapter.Chat(context.Background(), nil, llm.ChatOptions{})
	require.ErrorContains(t, err, "status 401")
	require.NotContains(t, err.Error(), "super-secret")
	require.Contains(t, err.Error(), "[REDACTED]")
	require.Equal(t, int32(1), calls.Load())
}

func TestChat_PropagatesCancellation(t *testing.T) {
	t.Parallel()
	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		close(started)
		<-release
	}))
	defer server.Close()
	adapter, err := NewAdapter(server.URL, "model",
		WithHTTPClient(server.Client()), WithAPIKeyLookup(staticKey("key")))
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-started
		cancel()
	}()

	_, err = adapter.Chat(ctx, nil, llm.ChatOptions{})
	close(release)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}

func writeResponse(t *testing.T, w http.ResponseWriter, blocks []contentBlock, input, output int) {
	t.Helper()
	response := chatResponse{}
	response.Message.Content = blocks
	response.Usage.Tokens.Input = input
	response.Usage.Tokens.Output = output
	w.Header().Set("Content-Type", "application/json")
	require.NoError(t, json.NewEncoder(w).Encode(response))
}

func chatServerError(t *testing.T, status int, body, key string) error {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()
	adapter, err := NewAdapter(server.URL, "model",
		WithHTTPClient(server.Client()), WithAPIKeyLookup(staticKey(key)))
	require.NoError(t, err)
	_, err = adapter.Chat(context.Background(), nil, llm.ChatOptions{})
	return err
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }
func (errorReader) Close() error             { return nil }

var _ io.ReadCloser = errorReader{}
