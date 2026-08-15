// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package profileaudit

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestInspectRESTRetryAggregate(t *testing.T) {
	maxDuration := time.Duration(1<<63 - 1)
	tests := []struct {
		name           string
		commandTimeout string
		requestTimeout string
		policy         string
		want           time.Duration
		wantDiagnostic string
	}{
		{
			name: "one attempt", commandTimeout: "3s", requestTimeout: "2s",
			policy: "{attempts: 1, backoff: none}", want: 2 * time.Second,
		},
		{
			name: "fixed below envelope", commandTimeout: "6s", requestTimeout: "2s",
			policy: "{attempts: 2, backoff: fixed, initial_delay: 1s}", want: 5 * time.Second,
		},
		{
			name: "exponential capped below envelope", commandTimeout: "10s", requestTimeout: "1s",
			policy: "{attempts: 4, backoff: exponential, initial_delay: 1s, max_delay: 2s}",
			want:   9 * time.Second,
		},
		{
			name: "aggregate equals envelope", commandTimeout: "5s", requestTimeout: "2s",
			policy:         "{attempts: 2, backoff: fixed, initial_delay: 1s}",
			want:           5 * time.Second,
			wantDiagnostic: "strictly below",
		},
		{
			name: "aggregate exceeds envelope", commandTimeout: "4s", requestTimeout: "2s",
			policy:         "{attempts: 2, backoff: fixed, initial_delay: 1s}",
			want:           5 * time.Second,
			wantDiagnostic: "strictly below",
		},
		{
			name:           "aggregate overflows",
			commandTimeout: maxDuration.String(),
			requestTimeout: (maxDuration/2 + 1).String(),
			policy:         "{attempts: 2, backoff: none}",
			wantDiagnostic: "no finite duration",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := writeRESTRetryProfile(
				t, tt.commandTimeout, "rest_client_invoke",
				tt.requestTimeout, "selected", tt.policy, "",
			)
			report, err := Inspect(profile)
			require.NoError(t, err)
			aggregate := requireRetryAggregate(t, report)
			require.Equal(t, tt.want, aggregate.Duration)
			require.Contains(t, aggregate.Authority, "REST client api retry selected aggregate")
			require.Contains(t, aggregate.Authority, "attempts")
			if tt.wantDiagnostic == "" {
				require.Empty(t, retryDiagnosticReasons(report))
				return
			}
			require.Contains(t, retryDiagnosticReasons(report), tt.wantDiagnostic)
		})
	}
}

func TestInspectRESTRetryAggregateRejectsInvalidPolicies(t *testing.T) {
	tests := []struct {
		name     string
		retryRef string
		policy   string
		want     string
	}{
		{name: "missing reference", retryRef: "missing", want: "undefined retry policy"},
		{name: "zero attempts", retryRef: "selected", policy: "{attempts: 0}", want: "attempts must be positive"},
		{
			name: "malformed delay", retryRef: "selected",
			policy: "{attempts: 2, backoff: fixed, initial_delay: later}",
			want:   "initial_delay",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := writeRESTRetryProfile(
				t, "1m", "rest_client_invoke", "1s",
				tt.retryRef, tt.policy, "",
			)
			_, err := Inspect(profile)
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func TestInspectRESTRetryAggregateExcludesAsyncAwait(t *testing.T) {
	profile := writeRESTRetryProfile(
		t, "10s", "rest_client_await", "2s", "selected",
		"{attempts: 2, backoff: fixed, initial_delay: 1s}",
		"3s",
	)
	report, err := Inspect(profile)
	require.NoError(t, err)
	for _, operation := range report.Operations {
		require.NotContains(t, operation.Authority, " retry ")
	}
	require.Contains(t, operationAuthorities(report), "async.timeout")
}

func writeRESTRetryProfile(
	t *testing.T,
	commandTimeout, init, requestTimeout, retryRef, policy, asyncTimeout string,
) string {
	t.Helper()
	root := t.TempDir()
	write(t, root, "machine.yaml", oneActionMachine(commandTimeout, "call"))
	write(t, root, "tools.yaml", "tools: [call]\n")
	write(t, root, "declarations.yaml", declarations(fmt.Sprintf(`
  - name: call
    type: builtin
    init: %s
    category: boundary
    visibility: internal
    emits: [RESTResponded, RESTAwaitTimedOut, CommandError]
    config: {rest_ref: api, operation: fetch}
`, init)))
	retries := ""
	if policy != "" {
		retries = "\n  retry_policies:\n    selected: " + policy
	}
	async := ""
	if asyncTimeout != "" {
		async = fmt.Sprintf(`
          async:
            request_id: $.id
            timeout: %s
            state_retention: consume`, asyncTimeout)
	}
	write(t, root, "rest.yaml", fmt.Sprintf(`
rest:
  version: v1
  limits:
    bounded: {timeout: %s}%s
  clients:
    api:
      limits_ref: bounded
      retry_ref: %s
      operations:
        fetch:
          method: GET
          path: /fetch
          success: {status: [200], signal: RESTResponded}%s
          side_effects:
            - {kind: external_api, target: fixture, state: read_only}
          reversibility: {classification: reversible, undo: noop}
`, requestTimeout, retries, retryRef, async))
	return writeProfile(
		t, root, "profile.yaml", "machine.yaml", "tools.yaml", "declarations.yaml",
		"rest_definitions: [rest.yaml]\n",
	)
}

func requireRetryAggregate(t *testing.T, report Report) Operation {
	t.Helper()
	for _, operation := range report.Operations {
		if strings.Contains(operation.Authority, " retry ") {
			return operation
		}
	}
	t.Fatalf("inspection report has no retry aggregate: %#v", report.Operations)
	return Operation{}
}

func retryDiagnosticReasons(report Report) string {
	var reasons []string
	for _, diagnostic := range report.Diagnostics {
		if strings.Contains(diagnostic.Authority, " retry ") {
			reasons = append(reasons, diagnostic.Authority+": "+diagnostic.Reason)
		}
	}
	return strings.Join(reasons, "; ")
}

func operationAuthorities(report Report) string {
	var authorities []string
	for _, operation := range report.Operations {
		authorities = append(authorities, operation.Authority)
	}
	return strings.Join(authorities, "; ")
}
