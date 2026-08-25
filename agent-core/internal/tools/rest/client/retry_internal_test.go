// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
