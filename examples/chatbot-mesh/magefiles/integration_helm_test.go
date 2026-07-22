// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestEnsureKindClusterTracksOwnership(t *testing.T) {
	t.Parallel()
	createErr := errors.New("create failed")
	tests := []struct {
		name        string
		exists      bool
		createErr   error
		wantCreated bool
		wantErr     bool
	}{
		{name: "pre-existing", exists: true},
		{name: "created by run", wantCreated: true},
		{name: "create failed", createErr: createErr, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			created, err := ensureKindCluster(
				"cluster",
				func(string) bool { return tt.exists },
				func(string) error { return tt.createErr },
			)
			if created != tt.wantCreated || (err != nil) != tt.wantErr {
				t.Fatalf("ensureKindCluster() = (%t, %v), want created=%t err=%t", created, err, tt.wantCreated, tt.wantErr)
			}
			if tt.createErr != nil && !errors.Is(err, tt.createErr) {
				t.Fatalf("ensureKindCluster() error = %v, want wrapped create error", err)
			}
		})
	}
}

func TestDeleteOwnedKindClusterNeverDeletesPreExisting(t *testing.T) {
	t.Parallel()
	var deleted []string
	deleteFn := func(name string) { deleted = append(deleted, name) }
	deleteOwnedKindCluster(false, "pre-existing", deleteFn)
	deleteOwnedKindCluster(true, "created", deleteFn)
	if len(deleted) != 1 || deleted[0] != "created" {
		t.Fatalf("deleted clusters = %v, want [created]", deleted)
	}
}

func TestSplitImageRef(t *testing.T) {
	cases := []struct {
		image, repo, tag string
	}{
		{"declarative-agents/agent-core:smoke", "declarative-agents/agent-core", "smoke"},
		{"ghcr.io/nokia-bell-labs/agent-core:0.1.0", "ghcr.io/nokia-bell-labs/agent-core", "0.1.0"},
		{"agent-core", "agent-core", "latest"},
		{"localhost:5000/agent-core:dev", "localhost:5000/agent-core", "dev"},
	}
	for _, c := range cases {
		repo, tag := splitImageRef(c.image)
		if repo != c.repo || tag != c.tag {
			t.Errorf("splitImageRef(%q) = (%q, %q), want (%q, %q)", c.image, repo, tag, c.repo, c.tag)
		}
	}
}

func TestJaegerAgentServicesExcludesJaeger(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/services" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"data":["chatbot","rag0","jaeger-all-in-one"]}`))
	}))
	defer srv.Close()

	n, services, err := jaegerAgentServices(srv.URL)
	if err != nil {
		t.Fatalf("jaegerAgentServices: %v", err)
	}
	if n != 2 {
		t.Fatalf("agent service count = %d, want 2 (services=%v)", n, services)
	}
	for _, s := range services {
		if s == "jaeger-all-in-one" {
			t.Fatalf("jaeger internal service should be excluded, got %v", services)
		}
	}
}

func TestAssertSmokeSpansBelowThreshold(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":["chatbot","jaeger"]}`))
	}))
	defer srv.Close()

	// Only one agent service is present, so the >=2 assertion must fail after a
	// short retry budget rather than hang.
	if err := assertSmokeSpans(srv.URL, 2, 200*time.Millisecond); err == nil {
		t.Fatal("assertSmokeSpans should fail when fewer than minServices agent services report")
	}
}

func TestAssertSmokeChatServedRejectsEmptyAnswer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"answer":"   "}`))
	}))
	defer srv.Close()

	if err := assertSmokeChatServed(srv.URL); err == nil {
		t.Fatal("assertSmokeChatServed should reject an empty answer")
	}
}

func TestAssertSmokeChatServedAcceptsAnswer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"answer":"The Northwind array is rated at 55 MW."}`))
	}))
	defer srv.Close()

	if err := assertSmokeChatServed(srv.URL); err != nil {
		t.Fatalf("assertSmokeChatServed rejected a served answer: %v", err)
	}
}

func TestHelmSmokeSkipReasonMissingBinary(t *testing.T) {
	// With an empty PATH none of the required binaries resolve, so the smoke test
	// records a skip for the first missing tool rather than attempting a run.
	t.Setenv("PATH", "")
	if reason := helmSmokeSkipReason(t.TempDir(), t.TempDir()); reason == "" {
		t.Fatal("helmSmokeSkipReason should report a skip when required binaries are absent")
	}
}
