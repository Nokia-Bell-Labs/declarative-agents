// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package rest

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	}, nil, nil)
	require.ErrorContains(t, err, `undefined retry policy "missing"`)
}
