// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package rest

import (
	"testing"
	"time"

	restdef "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/definition"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetryAggregateTimeout(t *testing.T) {
	t.Parallel()
	maxDuration := time.Duration(1<<63 - 1)
	tests := []struct {
		name           string
		attemptTimeout time.Duration
		retry          restdef.RetryPolicy
		want           time.Duration
		wantErr        string
	}{
		{
			name:           "one attempt",
			attemptTimeout: 2 * time.Second,
			retry:          restdef.RetryPolicy{Attempts: 1, Backoff: "none"},
			want:           2 * time.Second,
		},
		{
			name:           "fixed delays",
			attemptTimeout: 2 * time.Second,
			retry:          restdef.RetryPolicy{Attempts: 3, Backoff: "fixed", InitialDelay: "1s"},
			want:           8 * time.Second,
		},
		{
			name:           "exponential capped delays",
			attemptTimeout: time.Second,
			retry: restdef.RetryPolicy{
				Attempts: 4, Backoff: "exponential",
				InitialDelay: "1s", MaxDelay: "2s",
			},
			want: 9 * time.Second,
		},
		{
			name:           "none ignores declared delay",
			attemptTimeout: time.Second,
			retry:          restdef.RetryPolicy{Attempts: 3, Backoff: "none", InitialDelay: "1h"},
			want:           3 * time.Second,
		},
		{
			name:           "attempt product overflows",
			attemptTimeout: maxDuration/2 + 1,
			retry:          restdef.RetryPolicy{Attempts: 2, Backoff: "none"},
			wantErr:        "duration overflow",
		},
		{
			name:           "delay sum overflows",
			attemptTimeout: time.Second,
			retry: restdef.RetryPolicy{
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
