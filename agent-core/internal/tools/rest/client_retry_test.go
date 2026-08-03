// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		{name: "fixed is flat", retry: RetryPolicy{Backoff: "fixed", InitialDelay: "100ms"}, attempt: 4, want: 100 * time.Millisecond},
		{name: "exponential first attempt", retry: RetryPolicy{Backoff: "exponential", InitialDelay: "100ms"}, attempt: 1, want: 100 * time.Millisecond},
		{name: "exponential doubles", retry: RetryPolicy{Backoff: "exponential", InitialDelay: "100ms"}, attempt: 3, want: 400 * time.Millisecond},
		{name: "exponential capped by max_delay", retry: RetryPolicy{Backoff: "exponential", InitialDelay: "100ms", MaxDelay: "250ms"}, attempt: 5, want: 250 * time.Millisecond},
		{name: "flat capped by max_delay", retry: RetryPolicy{Backoff: "fixed", InitialDelay: "1s", MaxDelay: "250ms"}, attempt: 2, want: 250 * time.Millisecond},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, retryDelay(tt.retry, tt.attempt))
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

// Unsupported backoff and unparseable delays are rejected at load (GH-1379).
func TestValidateRetryPolicies_RejectsBadFields(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		retry   RetryPolicy
		errText string
	}{
		{name: "unknown backoff", retry: RetryPolicy{Backoff: "linear"}, errText: "unsupported backoff"},
		{name: "bad initial_delay", retry: RetryPolicy{InitialDelay: "soon"}, errText: "initial_delay"},
		{name: "bad max_delay", retry: RetryPolicy{Backoff: "exponential", MaxDelay: "later"}, errText: "max_delay"},
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
