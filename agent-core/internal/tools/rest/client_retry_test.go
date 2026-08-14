// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestRetryAggregateTimeout(t *testing.T) {
	t.Parallel()
	maxDuration := time.Duration(1<<63 - 1)
	tests := []struct {
		name           string
		attemptTimeout time.Duration
		retry          RetryPolicy
		want           time.Duration
		wantErr        string
	}{
		{
			name:           "one attempt",
			attemptTimeout: 2 * time.Second,
			retry:          RetryPolicy{Attempts: 1, Backoff: "none"},
			want:           2 * time.Second,
		},
		{
			name:           "fixed delays",
			attemptTimeout: 2 * time.Second,
			retry:          RetryPolicy{Attempts: 3, Backoff: "fixed", InitialDelay: "1s"},
			want:           8 * time.Second,
		},
		{
			name:           "exponential capped delays",
			attemptTimeout: time.Second,
			retry: RetryPolicy{
				Attempts: 4, Backoff: "exponential",
				InitialDelay: "1s", MaxDelay: "2s",
			},
			want: 9 * time.Second,
		},
		{
			name:           "none ignores declared delay",
			attemptTimeout: time.Second,
			retry:          RetryPolicy{Attempts: 3, Backoff: "none", InitialDelay: "1h"},
			want:           3 * time.Second,
		},
		{
			name:           "attempt product overflows",
			attemptTimeout: maxDuration/2 + 1,
			retry:          RetryPolicy{Attempts: 2, Backoff: "none"},
			wantErr:        "duration overflow",
		},
		{
			name:           "delay sum overflows",
			attemptTimeout: time.Second,
			retry: RetryPolicy{
				Attempts: 2, Backoff: "fixed",
				InitialDelay: maxDuration.String(),
			},
			wantErr: "duration overflow",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := RetryAggregateTimeout(tt.attemptTimeout, tt.retry)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// sleepWithContext returns immediately when the context is already cancelled,
// so a cancelled run does not burn the delay (GH-1379).
func TestSleepWithContext_CancelledStopsWaiting(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	err := sleepWithContext(ctx, time.Hour)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Less(t, time.Since(start), 500*time.Millisecond)
}

func TestSleepWithContext_CompletesShortDelay(t *testing.T) {
	t.Parallel()
	require.NoError(t, sleepWithContext(context.Background(), 5*time.Millisecond))
}

func TestDoWithRetryReplaysBodyAndReturnsSecondResponse(t *testing.T) {
	t.Parallel()
	var bodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
			http.Error(w, "read body", http.StatusInternalServerError)
			return
		}
		bodies = append(bodies, string(body))
		if len(bodies) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	request, err := http.NewRequestWithContext(
		context.Background(), http.MethodPost, server.URL, strings.NewReader("payload"),
	)
	require.NoError(t, err)
	cmd := clientCmd{operation: ClientOperationDefinition{Retry: RetryPolicy{
		Attempts: 2, Backoff: "fixed", InitialDelay: "1ms",
		RetryStatus: []int{http.StatusServiceUnavailable},
	}}}

	response, attempts, err := cmd.doWithRetry(request)

	require.NoError(t, err)
	t.Cleanup(func() { _ = response.Body.Close() })
	assert.Equal(t, http.StatusOK, response.StatusCode)
	assert.Equal(t, 2, attempts)
	assert.Equal(t, []string{"payload", "payload"}, bodies)
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

// Unsupported backoff and unparseable delays are rejected at load (GH-1379).
func TestValidateRetryPolicies_RejectsBadFields(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		retry   RetryPolicy
		errText string
	}{
		{name: "unknown backoff", retry: RetryPolicy{Attempts: 1, Backoff: "linear"}, errText: "unsupported backoff"},
		{name: "bad initial_delay", retry: RetryPolicy{Attempts: 1, InitialDelay: "soon"}, errText: "initial_delay"},
		{name: "negative initial_delay", retry: RetryPolicy{Attempts: 1, InitialDelay: "-1ms"}, errText: "non-negative"},
		{name: "bad max_delay", retry: RetryPolicy{Attempts: 1, Backoff: "exponential", MaxDelay: "later"}, errText: "max_delay"},
		{name: "negative max_delay", retry: RetryPolicy{Attempts: 1, Backoff: "exponential", MaxDelay: "-1ms"}, errText: "non-negative"},
		{name: "zero attempts", retry: RetryPolicy{Attempts: 0}, errText: "positive"},
		{name: "negative attempts", retry: RetryPolicy{Attempts: -1}, errText: "attempts"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateRetryPolicies(map[string]RetryPolicy{"p": tt.retry})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errText)
		})
	}
	assert.NoError(t, validateRetryPolicies(map[string]RetryPolicy{
		"ok": {Backoff: "exponential", InitialDelay: "100ms", MaxDelay: "2s", Attempts: 3},
	}))
}

func TestValidateClientsRejectsMissingRetryPolicy(t *testing.T) {
	t.Parallel()
	err := validateClients(map[string]Client{
		"api": {RetryRef: "missing"},
	}, nil)
	require.ErrorContains(t, err, `undefined retry policy "missing"`)
}
