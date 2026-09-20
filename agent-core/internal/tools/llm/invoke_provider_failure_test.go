// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package llm

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	modelllm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/credentials"
)

// The chat failure taxonomy reaches the machine only when the ToolDef asks
// for it (srd058 R2.4, rel23.0-uc001).

func failureBuilder(t *testing.T, url string, optIn bool, timeout int) *InvokeLLMBuilder {
	t.Helper()
	config := map[string]interface{}{
		"dialect": fixtureDialect(t), "provider_url": url,
		"model": "fixture-large", "manifest_state": "Composing",
	}
	if optIn {
		config["provider_failure_signals"] = true
	}
	if timeout > 0 {
		config["llm_timeout"] = timeout
	}
	builder, err := NewInvokeLLMBuilder(
		catalog.ToolDef{Name: "invoke_llm", Config: config},
		InvokeLLMFactoryDeps{
			History:  modelllm.NewConversation(nil, "", modelllm.ChatOptions{}),
			Registry: core.NewRegistry(), Tracer: tracing.NewRecordingTracer(),
			Ctx:         context.Background(),
			Credentials: credentials.Static{"FIXTURE_API_KEY": fixtureToken},
		})
	require.NoError(t, err)
	return builder
}

func answering(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server
}

func TestInvokeLLMEmitsMappedProviderFailure(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		status int
		want   core.Signal
	}{
		{http.StatusServiceUnavailable, "ProviderUnavailable"},
		{http.StatusTooManyRequests, "ProviderThrottled"},
		{http.StatusUnauthorized, "ProviderUnauthorized"},
		{http.StatusForbidden, "ProviderUnauthorized"},
	} {
		server := answering(t, tc.status, `{"error":"no"}`)
		result := failureBuilder(t, server.URL, true, 0).
			Build(core.Result{State: "Composing", Output: "hi"}).Execute()
		require.Equal(t, tc.want, result.Signal, "status %d", tc.status)
		require.Error(t, result.Err)
	}
}

func TestInvokeLLMEmitsUnavailableOnTransportFailure(t *testing.T) {
	t.Parallel()

	t.Run("refused connection", func(t *testing.T) {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		url := "http://" + listener.Addr().String()
		require.NoError(t, listener.Close()) // nothing listens there now

		result := failureBuilder(t, url, true, 0).
			Build(core.Result{State: "Composing", Output: "hi"}).Execute()
		require.Equal(t, core.Signal("ProviderUnavailable"), result.Signal)
	})

	// The failure that motivated the release: a provider that accepts the
	// connection and never answers carries no status to map (GH-2297).
	t.Run("deadline exceeded", func(t *testing.T) {
		// The handler must be released before the server is closed: Close
		// waits for outstanding requests, and a cleanup registered after it
		// would run too late (Cleanup is LIFO).
		blocked := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			<-blocked
		}))
		t.Cleanup(server.Close)
		t.Cleanup(func() { close(blocked) })

		started := time.Now()
		result := failureBuilder(t, server.URL, true, 1).
			Build(core.Result{State: "Composing", Output: "hi"}).Execute()
		require.Equal(t, core.Signal("ProviderUnavailable"), result.Signal)
		require.Less(t, time.Since(started), 30*time.Second, "the call is bounded by its timeout")
	})
}

func TestInvokeLLMKeepsCommandErrorForUnclassifiedFailures(t *testing.T) {
	t.Parallel()

	t.Run("unmapped status", func(t *testing.T) {
		// 418 is an error the fixture dialect's failure map does not name.
		server := answering(t, http.StatusTeapot, `{"error":"no"}`)
		result := failureBuilder(t, server.URL, true, 0).
			Build(core.Result{State: "Composing", Output: "hi"}).Execute()
		require.Equal(t, core.CommandError, result.Signal)
	})

	t.Run("credential fault", func(t *testing.T) {
		server := answering(t, http.StatusOK, `{}`)
		builder, err := NewInvokeLLMBuilder(
			catalog.ToolDef{Name: "invoke_llm", Config: map[string]interface{}{
				"dialect": fixtureDialect(t), "provider_url": server.URL,
				"model": "fixture-large", "manifest_state": "Composing",
				"provider_failure_signals": true,
			}},
			InvokeLLMFactoryDeps{
				History:  modelllm.NewConversation(nil, "", modelllm.ChatOptions{}),
				Registry: core.NewRegistry(), Tracer: tracing.NewRecordingTracer(),
				Ctx: context.Background(), Credentials: credentials.Static{},
			})
		require.NoError(t, err)
		result := builder.Build(core.Result{State: "Composing", Output: "hi"}).Execute()
		require.Equal(t, core.CommandError, result.Signal,
			"an unresolvable credential is a configuration fault, not a provider state")
	})

	t.Run("decode fault", func(t *testing.T) {
		server := answering(t, http.StatusOK, `not json at all`)
		result := failureBuilder(t, server.URL, true, 0).
			Build(core.Result{State: "Composing", Output: "hi"}).Execute()
		require.Equal(t, core.CommandError, result.Signal)
	})
}

func TestInvokeLLMDefaultsToCommandError(t *testing.T) {
	t.Parallel()
	for _, status := range []int{
		http.StatusServiceUnavailable, http.StatusTooManyRequests, http.StatusUnauthorized,
	} {
		server := answering(t, status, `{"error":"no"}`)
		result := failureBuilder(t, server.URL, false, 0).
			Build(core.Result{State: "Composing", Output: "hi"}).Execute()
		require.Equal(t, core.CommandError, result.Signal,
			"without the opt-in status %d emits what it emitted before", status)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	url := "http://" + listener.Addr().String()
	require.NoError(t, listener.Close())
	result := failureBuilder(t, url, false, 0).
		Build(core.Result{State: "Composing", Output: "hi"}).Execute()
	require.Equal(t, core.CommandError, result.Signal)
}
