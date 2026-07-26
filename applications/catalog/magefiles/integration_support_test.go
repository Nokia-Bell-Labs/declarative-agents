// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWaitHTTPStatusBoundsStalledRequestByOuterDeadline(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
		<-req.Context().Done()
	}))
	defer server.Close()

	start := time.Now()
	err := waitHTTPStatusWithClient(&http.Client{}, server.URL, http.StatusOK, 100*time.Millisecond)

	if err == nil {
		t.Fatal("waitHTTPStatusWithClient() expected timeout")
	}
	if elapsed := time.Since(start); elapsed >= 500*time.Millisecond {
		t.Fatalf("waitHTTPStatusWithClient() elapsed %s, outer deadline was 100ms", elapsed)
	}
}

func TestWaitHTTPStatusPreservesLastTransportError(t *testing.T) {
	t.Parallel()
	transportErr := errors.New("injected transport failure")
	client := &http.Client{Transport: profileRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, transportErr
	})}

	err := waitHTTPStatusWithClient(client, "http://integration.invalid", http.StatusOK, 120*time.Millisecond)

	if !errors.Is(err, transportErr) {
		t.Fatalf("waitHTTPStatusWithClient() error = %v, want wrapped transport error", err)
	}
}

func TestWaitHTTPStatusReturnsOnExpectedStatus(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	if err := waitHTTPStatusWithClient(&http.Client{}, server.URL, http.StatusAccepted, time.Second); err != nil {
		t.Fatalf("waitHTTPStatusWithClient() error: %v", err)
	}
}

type profileRoundTripFunc func(*http.Request) (*http.Response, error)

func (f profileRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
