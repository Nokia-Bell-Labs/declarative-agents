// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package client

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

func (c clientCmd) doWithRetry(request *http.Request) (*http.Response, int, error) {
	client := httpClient(c.operation.Limits)
	attempts := retryAttempts(c.operation.Retry)
	ctx := request.Context()
	for attempt := 1; attempt <= attempts; attempt++ {
		response, err := client.Do(cloneRequest(request))
		if shouldReturnResponse(response, err, attempt, attempts, c.operation.Retry) {
			return response, attempt, err
		}
		closeResponse(response)
		if err := sleepWithContext(ctx, RetryDelay(c.operation.Retry, attempt)); err != nil {
			return nil, attempt, err
		}
	}
	return nil, attempts, fmt.Errorf("REST request failed after %d attempts", attempts)
}

// retryDelay computes the wait before the next attempt from the declared
// backoff. srd028 R5.8 sanctions transport-declared retries, so the declared
// backoff and max_delay are honored rather than ignored (GH-1379): none waits
// zero, fixed (and the legacy empty default) use initial_delay, and exponential
// doubles initial_delay with overflow-safe saturation at max_delay.
func RetryDelay(retry RetryPolicy, attempt int) time.Duration {
	if retry.Backoff == "none" {
		return 0
	}
	initial := parseDuration(retry.InitialDelay, 0)
	maxDelay := parseDuration(retry.MaxDelay, 0)
	if retry.Backoff != "exponential" {
		return capRetryDelay(initial, maxDelay)
	}
	return exponentialRetryDelay(initial, maxDelay, attempt)
}

func exponentialRetryDelay(initial, maximum time.Duration, attempt int) time.Duration {
	delay := capRetryDelay(initial, maximum)
	if delay <= 0 || attempt <= 1 || maximum > 0 && delay >= maximum {
		return delay
	}
	const maxDuration = time.Duration(1<<63 - 1)
	for step := 1; step < attempt; step++ {
		if maximum > 0 && delay > maximum/2 {
			return maximum
		}
		if delay > maxDuration/2 {
			return capRetryDelay(maxDuration, maximum)
		}
		delay *= 2
	}
	return capRetryDelay(delay, maximum)
}

func capRetryDelay(delay, maximum time.Duration) time.Duration {
	if maximum > 0 && delay > maximum {
		return maximum
	}
	return delay
}

// sleepWithContext waits for d unless the dispatch context is cancelled first,
// so a cancelled run stops burning the retry delay instead of blocking on a
// bare time.Sleep (GH-1379).
func sleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func httpClient(limits LimitProfile) *http.Client {
	client := &http.Client{
		Timeout:   parseDuration(limits.Timeout, 0),
		Transport: networkPolicyTransport(limits.Network, net.DefaultResolver, (&net.Dialer{}).DialContext),
	}
	client.CheckRedirect = redirectPolicy(limits)
	return client
}

type ipAddrResolver interface {
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}

type contextDialer func(context.Context, string, string) (net.Conn, error)

func networkPolicyTransport(
	policy NetworkPolicy,
	resolver ipAddrResolver,
	dial contextDialer,
) http.RoundTripper {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if len(policy.CIDRs) == 0 {
		return transport
	}
	// A proxy would make DialContext validate the proxy rather than the declared
	// destination. CIDR-restricted operations therefore connect directly.
	transport.Proxy = nil
	transport.DialContext = cidrPolicyDialer(policy, resolver, dial)
	return transport
}

func cidrPolicyDialer(
	policy NetworkPolicy,
	resolver ipAddrResolver,
	dial contextDialer,
) contextDialer {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("CIDR policy split address %q: %w", address, err)
		}
		ips, err := resolveDialIPs(ctx, resolver, host)
		if err != nil {
			return nil, err
		}
		for _, ip := range ips {
			if !ipAllowedByCIDR(ip, policy.CIDRs) {
				return nil, networkPolicyError{
					error: fmt.Errorf("host %q resolves outside CIDR policy: %s", host, ip),
				}
			}
		}
		var dialErr error
		for _, ip := range ips {
			conn, err := dial(ctx, network, net.JoinHostPort(ip.String(), port))
			if err == nil {
				return conn, nil
			}
			dialErr = errors.Join(dialErr, err)
		}
		return nil, fmt.Errorf("dial allowed host %q: %w", host, dialErr)
	}
}

func resolveDialIPs(ctx context.Context, resolver ipAddrResolver, host string) ([]net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		return []net.IP{ip}, nil
	}
	addrs, err := resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("resolve host %q for CIDR policy: %w", host, err)
	}
	ips := make([]net.IP, 0, len(addrs))
	for _, addr := range addrs {
		ips = append(ips, addr.IP)
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("host %q resolved to no addresses", host)
	}
	return ips, nil
}

func redirectPolicy(limits LimitProfile) func(*http.Request, []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		policy := limits.Redirect
		if policy.Mode == redirectNone || policy.Mode == "" {
			return http.ErrUseLastResponse
		}
		if err := validateNetwork(req.URL, limits.Network); err != nil {
			return wrapNetworkPolicyError(err)
		}
		if policy.Mode == redirectSameHost && len(via) > 0 && req.URL.Host != via[0].URL.Host {
			return http.ErrUseLastResponse
		}
		if policy.Mode == redirectAllowlist && !stringIn(req.URL.Hostname(), policy.AllowHosts) {
			return fmt.Errorf("redirect host %q is not allowed", req.URL.Hostname())
		}
		if policy.MaxRedirects > 0 && len(via) >= policy.MaxRedirects {
			return http.ErrUseLastResponse
		}
		return nil
	}
}

func retryAttempts(policy RetryPolicy) int {
	if policy.Attempts > 0 {
		return policy.Attempts
	}
	return 1
}

func shouldReturnResponse(response *http.Response, err error, attempt, max int, retry RetryPolicy) bool {
	if attempt >= max {
		return true
	}
	if err != nil {
		return !retry.RetryNetworkErrors
	}
	return !statusIn(response.StatusCode, retry.RetryStatus)
}

func closeResponse(response *http.Response) {
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
}

func cloneRequest(request *http.Request) *http.Request {
	clone := request.Clone(request.Context())
	if request.GetBody != nil {
		body, _ := request.GetBody()
		clone.Body = body
	}
	return clone
}
