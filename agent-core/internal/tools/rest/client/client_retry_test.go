// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package client_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// retryDelay must read the declared backoff and max_delay, not just
// initial_delay (GH-1379).
func TestRetryDelay_HonorsDeclaredBackoff(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		retry   RetryPolicy
		attempt int
		want    time.Duration
	}{
		{name: "empty backoff is flat", retry: RetryPolicy{InitialDelay: "100ms"}, attempt: 3, want: 100 * time.Millisecond},
		{name: "none has no delay", retry: RetryPolicy{Backoff: "none", InitialDelay: "100ms"}, attempt: 3, want: 0},
		{name: "fixed is flat", retry: RetryPolicy{Backoff: "fixed", InitialDelay: "100ms"}, attempt: 4, want: 100 * time.Millisecond},
		{name: "exponential first attempt", retry: RetryPolicy{Backoff: "exponential", InitialDelay: "100ms"}, attempt: 1, want: 100 * time.Millisecond},
		{name: "exponential doubles", retry: RetryPolicy{Backoff: "exponential", InitialDelay: "100ms"}, attempt: 3, want: 400 * time.Millisecond},
		{name: "exponential capped by max_delay", retry: RetryPolicy{Backoff: "exponential", InitialDelay: "100ms", MaxDelay: "250ms"}, attempt: 5, want: 250 * time.Millisecond},
		{name: "exponential saturates before overflow", retry: RetryPolicy{Backoff: "exponential", InitialDelay: "1ns"}, attempt: 1_000, want: time.Duration(1<<63 - 1)},
		{name: "flat capped by max_delay", retry: RetryPolicy{Backoff: "fixed", InitialDelay: "1s", MaxDelay: "250ms"}, attempt: 2, want: 250 * time.Millisecond},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, retryDelay(tt.retry, tt.attempt))
		})
	}
}

func TestRESTClientDispatchCancellationStopsBackoff(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	firstRequest := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) == 1 {
			close(firstRequest)
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer upstream.Close()
	def := clientDefinition(t, upstream.URL, issueClient())
	def.RetryPolicies = map[string]RetryPolicy{"slow": {
		Attempts: 2, Backoff: "fixed", InitialDelay: "1h",
		RetryStatus: []int{http.StatusServiceUnavailable},
	}}
	client := def.Clients["github"]
	client.RetryRef = "slow"
	def.Clients["github"] = client
	require.NoError(t, ValidateDefinition(def))
	command := clientCommand(t, def, InitClientGet, "get", params("1"))
	ctx, cancel := context.WithCancel(context.Background())
	results := make(chan core.Result, 1)
	go func() { results <- core.SafeExecuteContext(ctx, command, 0) }()
	<-firstRequest
	start := time.Now()
	cancel()
	result := <-results

	require.Equal(t, core.CommandError, result.Signal)
	require.ErrorContains(t, result.Err, "cancelled executing")
	require.Less(t, time.Since(start), time.Second)
	require.Equal(t, int32(1), requests.Load())
}

func TestRESTClientDispatchCancellationAbortsInFlightRequest(t *testing.T) {
	t.Parallel()

	handlerStarted := make(chan struct{})
	releaseHandler := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		close(handlerStarted)
		<-releaseHandler
	}))
	defer upstream.Close()
	def := clientDefinition(t, upstream.URL, issueClient())
	command := clientCommand(t, def, InitClientGet, "get", params("1"))
	ctx, cancel := context.WithCancel(context.Background())
	results := make(chan core.Result, 1)
	go func() { results <- core.SafeExecuteContext(ctx, command, 0) }()
	<-handlerStarted
	cancel()
	result := <-results
	close(releaseHandler)

	require.Equal(t, core.CommandError, result.Signal)
	require.ErrorContains(t, result.Err, "cancelled executing")
}

func TestRESTClientCancelledMutationReportsIndeterminate(t *testing.T) {
	t.Parallel()

	handlerStarted := make(chan struct{})
	releaseHandler := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		close(handlerStarted)
		<-releaseHandler
	}))
	defer upstream.Close()
	def := clientDefinition(t, upstream.URL, issueClient())
	command := clientCommand(t, def, InitClientSet, "set", params("1", "changed"))
	contextual, ok := command.(core.ContextCommand)
	require.True(t, ok)
	ctx, cancel := context.WithCancel(context.Background())
	results := make(chan core.Result, 1)
	go func() { results <- contextual.ExecuteContext(ctx) }()
	<-handlerStarted
	cancel()

	result := <-results
	close(releaseHandler)

	require.Equal(t, core.CommandError, result.Signal)
	require.Contains(t, result.Output, `"outcome":"indeterminate"`)
}

func TestRESTClientExecuteRetainsBackgroundContextCompatibility(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]interface{}{"id": "1"})
	}))
	defer upstream.Close()
	def := clientDefinition(t, upstream.URL, issueClient())

	result := clientCommand(t, def, InitClientGet, "get", params("1")).Execute()

	require.Equal(t, core.Signal("RESTResourceRead"), result.Signal, result.Output)
}
