// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"context"
	"errors"
	"net"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

type staticIPResolver struct {
	addrs []net.IPAddr
	err   error
}

func (r staticIPResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return r.addrs, r.err
}

func TestCIDRPolicyDialerValidatesEveryResolvedAddress(t *testing.T) {
	tests := []struct {
		name    string
		cidrs   []string
		addrs   []net.IPAddr
		wantErr string
	}{
		{name: "allowed IPv4", cidrs: []string{"192.0.2.0/24"}, addrs: ipAddrs("192.0.2.10")},
		{name: "allowed IPv6", cidrs: []string{"2001:db8::/32"}, addrs: ipAddrs("2001:db8::10")},
		{
			name: "mixed answer", cidrs: []string{"192.0.2.0/24"},
			addrs: ipAddrs("192.0.2.10", "127.0.0.1"), wantErr: "resolves outside CIDR policy",
		},
		{name: "all disallowed", cidrs: []string{"192.0.2.0/24"}, addrs: ipAddrs("127.0.0.1"), wantErr: "resolves outside CIDR policy"},
		{name: "empty answer", cidrs: []string{"192.0.2.0/24"}, wantErr: "resolved to no addresses"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var dialed string
			dial := cidrPolicyDialer(
				NetworkPolicy{CIDRs: tc.cidrs},
				staticIPResolver{addrs: tc.addrs},
				func(_ context.Context, _ string, address string) (net.Conn, error) {
					dialed = address
					client, server := net.Pipe()
					t.Cleanup(func() {
						_ = client.Close()
						_ = server.Close()
					})
					return client, nil
				},
			)
			conn, err := dial(context.Background(), "tcp", "service.test:8443")
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				require.Empty(t, dialed, "policy rejection must happen before dialing")
				return
			}
			require.NoError(t, err)
			require.NotNil(t, conn)
			require.Equal(t, net.JoinHostPort(tc.addrs[0].IP.String(), "8443"), dialed)
		})
	}
}

func TestCIDRPolicyDialerRejectsResolutionErrorsAndRebinding(t *testing.T) {
	policy := NetworkPolicy{CIDRs: []string{"192.0.2.0/24"}}
	resolveErr := errors.New("DNS unavailable")
	dial := cidrPolicyDialer(policy, staticIPResolver{err: resolveErr},
		func(context.Context, string, string) (net.Conn, error) {
			t.Fatal("dial called after resolution failure")
			return nil, nil
		})
	_, err := dial(context.Background(), "tcp", "service.test:443")
	require.ErrorIs(t, err, resolveErr)

	responses := [][]net.IPAddr{ipAddrs("192.0.2.10"), ipAddrs("127.0.0.1")}
	resolver := &sequenceIPResolver{responses: responses}
	dial = cidrPolicyDialer(policy, resolver, func(_ context.Context, _, _ string) (net.Conn, error) {
		client, server := net.Pipe()
		t.Cleanup(func() {
			_ = client.Close()
			_ = server.Close()
		})
		return client, nil
	})
	conn, err := dial(context.Background(), "tcp", "service.test:443")
	require.NoError(t, err)
	require.NoError(t, conn.Close())
	_, err = dial(context.Background(), "tcp", "service.test:443")
	require.ErrorContains(t, err, "resolves outside CIDR policy")
}

func TestValidateNetworkLiteralAddressAndPortBoundaries(t *testing.T) {
	tests := []struct {
		name    string
		rawURL  string
		policy  NetworkPolicy
		wantErr string
	}{
		{
			name: "IPv4 and explicit port allowed", rawURL: "http://192.0.2.10:8080",
			policy: NetworkPolicy{CIDRs: []string{"192.0.2.0/24"}, Ports: []int{8080}},
		},
		{
			name: "IPv6 and default port allowed", rawURL: "https://[2001:db8::10]",
			policy: NetworkPolicy{CIDRs: []string{"2001:db8::/32"}, Ports: []int{443}},
		},
		{
			name: "default port rejected", rawURL: "https://192.0.2.10",
			policy:  NetworkPolicy{CIDRs: []string{"192.0.2.0/24"}, Ports: []int{8443}},
			wantErr: `port "443" is not allowed`,
		},
		{
			name: "explicit port rejected", rawURL: "http://192.0.2.10:8080",
			policy:  NetworkPolicy{CIDRs: []string{"192.0.2.0/24"}, Ports: []int{80}},
			wantErr: `port "8080" is not allowed`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			endpoint, err := url.Parse(tc.rawURL)
			require.NoError(t, err)
			err = validateNetwork(endpoint, tc.policy)
			if tc.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.wantErr)
			}
		})
	}
}

type sequenceIPResolver struct {
	responses [][]net.IPAddr
	index     int
}

func (r *sequenceIPResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	result := r.responses[r.index]
	r.index++
	return result, nil
}

func ipAddrs(raw ...string) []net.IPAddr {
	addrs := make([]net.IPAddr, 0, len(raw))
	for _, value := range raw {
		addrs = append(addrs, net.IPAddr{IP: net.ParseIP(value)})
	}
	return addrs
}
