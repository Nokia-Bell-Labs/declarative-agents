// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package client

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func discoveredPodState(host string) core.CommandStateView {
	return core.NewCommandStateView(core.Execution{{
		CommandName: "discover_pods",
		Result:      commandStateDigest(`{"ip":"` + host + `"}`),
	}})
}

// TestResolveOperationBaseURLComposesAuthority covers scheme and port
// composition, including the IPv6 literal and the omitted-port default.
func TestResolveOperationBaseURLComposesAuthority(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		host   string
		scheme string
		port   string
		want   string
	}{
		{name: "default scheme with port", host: "10.244.0.7", port: "18202", want: "http://10.244.0.7:18202"},
		{name: "declared https", host: "10.244.0.7", scheme: "https", port: "8443", want: "https://10.244.0.7:8443"},
		{name: "omitted port", host: "monitor.internal", want: "http://monitor.internal"},
		{name: "ipv6 with port", host: "fd00::1", port: "18202", want: "http://[fd00::1]:18202"},
		{name: "ipv6 without port", host: "fd00::1", want: "http://[fd00::1]"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			operation := Operation{
				BaseURLSource:       bodySourceCommandState,
				BaseURLHostSelector: "$from(discover_pods).ip",
				BaseURLScheme:       tc.scheme,
				BaseURLPort:         tc.port,
			}
			base, selected, err := resolveOperationBaseURL(operation, "http://127.0.0.1:1", discoveredPodState(tc.host))
			require.NoError(t, err)
			require.True(t, selected)
			require.Equal(t, tc.want, base)
		})
	}
}

// TestRESTClient_HostSelectorRejectionTable covers the load-time rejections for
// composed target configuration (srd028 R14.6).
