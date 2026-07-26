// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// swapIntegrationClients installs probe and inference clients for one test and
// returns the restore func. Tests using it must not run in parallel: the
// clients are package state. Swapping lets the routing be proved with
// millisecond bounds instead of a test that really waits out the 2s probe
// bound.
func swapIntegrationClients(probe, inference *http.Client) func() {
	priorProbe, priorInference := integrationHTTPClient, integrationInferenceClient
	integrationHTTPClient, integrationInferenceClient = probe, inference
	return func() {
		integrationHTTPClient, integrationInferenceClient = priorProbe, priorInference
	}
}

func sleepingServer(d time.Duration) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(d)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
}

// TestRequestInferenceOutlivesProbeBound is the GH-709 regression: model work
// that exceeds the probe bound must still complete. Before the split, an
// embedding slower than 2s failed the tracer outright.
func TestRequestInferenceOutlivesProbeBound(t *testing.T) {
	server := sleepingServer(150 * time.Millisecond)
	defer server.Close()
	restore := swapIntegrationClients(
		&http.Client{Timeout: 30 * time.Millisecond},
		&http.Client{Timeout: 5 * time.Second},
	)
	defer restore()

	if _, _, err := requestHTTP(http.MethodPost, server.URL, `{}`); err == nil {
		t.Fatal("requestHTTP() expected the probe bound to reject work slower than a health check")
	}

	_, status, err := requestInference(http.MethodPost, server.URL, `{}`, "embed query vector")
	if err != nil {
		t.Fatalf("requestInference() error: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("requestInference() status = %d, want %d", status, http.StatusOK)
	}
}

// TestProbeStillFailsFastWhenServiceIsDown covers GH-709 R4: the longer
// inference bound must not leak into probes, or an unreachable service would
// hang a tracer for minutes instead of failing in seconds.
func TestProbeStillFailsFastWhenServiceIsDown(t *testing.T) {
	server := sleepingServer(2 * time.Second)
	defer server.Close()
	restore := swapIntegrationClients(
		&http.Client{Timeout: 40 * time.Millisecond},
		&http.Client{Timeout: 5 * time.Second},
	)
	defer restore()

	started := time.Now()
	if _, _, err := requestHTTP(http.MethodPost, server.URL, `{}`); err == nil {
		t.Fatal("requestHTTP() expected a fast failure against an unresponsive service")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("requestHTTP() took %s; the probe bound must not follow the inference bound", elapsed)
	}
}

// TestInferenceTimeoutErrorNamesTheWork covers GH-709 R3. A bare
// "context deadline exceeded" cannot distinguish a slow model from a dead
// service, which is what sent the original investigation down the wrong path.
func TestInferenceTimeoutErrorNamesTheWork(t *testing.T) {
	server := sleepingServer(500 * time.Millisecond)
	defer server.Close()
	restore := swapIntegrationClients(
		&http.Client{Timeout: 30 * time.Millisecond},
		&http.Client{Timeout: 20 * time.Millisecond},
	)
	defer restore()

	_, _, err := requestInference(http.MethodPost, server.URL, `{}`, "embed query vector with model qwen3-embedding:8b")
	if err == nil {
		t.Fatal("requestInference() expected a timeout")
	}
	for _, want := range []string{
		"embed query vector with model qwen3-embedding:8b",
		server.URL,
		"inference timeout",
		integrationInferenceTimeoutEnv,
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("requestInference() error = %q, want it to name %q", err, want)
		}
	}
}

// TestInferenceClientOutranksProbeClient guards the wiring itself: whatever the
// environment says, the shipped inference client must never be bounded as
// tightly as a probe.
func TestInferenceClientOutranksProbeClient(t *testing.T) {
	if integrationInferenceClient.Timeout <= integrationHTTPClient.Timeout {
		t.Fatalf("inference client timeout %s must exceed probe timeout %s",
			integrationInferenceClient.Timeout, integrationHTTPClient.Timeout)
	}
	if integrationInferenceClient.Timeout < integrationHTTPRequestTimeout*10 {
		t.Fatalf("inference client timeout %s is too close to the probe bound %s to survive a cold model load",
			integrationInferenceClient.Timeout, integrationHTTPRequestTimeout)
	}
}

func TestIntegrationInferenceTimeoutReadsOverride(t *testing.T) {
	t.Setenv(integrationInferenceTimeoutEnv, "45s")

	if got := integrationInferenceTimeout(); got != 45*time.Second {
		t.Fatalf("integrationInferenceTimeout() = %s, want 45s", got)
	}
}

// TestIntegrationInferenceTimeoutRejectsUnusableValues checks that a bad
// override falls back to the default. Falling back to zero would mean "no
// timeout" and falling back to the probe bound would resurrect GH-709.
func TestIntegrationInferenceTimeoutRejectsUnusableValues(t *testing.T) {
	for _, value := range []string{"", "   ", "not-a-duration", "0s", "-5s"} {
		t.Setenv(integrationInferenceTimeoutEnv, value)

		if got := integrationInferenceTimeout(); got != integrationInferenceTimeoutDefault {
			t.Errorf("integrationInferenceTimeout() with %q = %s, want the %s default",
				value, got, integrationInferenceTimeoutDefault)
		}
	}
}
