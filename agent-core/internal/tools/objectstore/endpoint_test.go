// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package objectstore

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// srd059 R2.2, AC2: the explicit endpoint field is the one emulator path.
// The decoy environment variable points at a port nothing listens on, so a
// code path honoring it would fail here instead of reaching the configured
// server (srd059's answer to the STORAGE_EMULATOR_HOST convention).
func TestExplicitEndpoint(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("from the configured endpoint"))
	}))
	t.Cleanup(server.Close)
	t.Setenv("STORAGE_EMULATOR_HOST", "127.0.0.1:1")

	opener := NewOpener()
	connection := ConnectionConfig{BucketURL: "gs://agents", Endpoint: server.URL}
	read := (&ReadBuilder{Opener: opener, Connection: connection}).
		Build(paramsResult(map[string]string{"key": "anything.txt"})).Execute()
	if read.Signal != core.ToolDone {
		t.Fatalf("read through explicit endpoint: %v %s", read.Signal, read.Output)
	}
	if !strings.Contains(read.Output, "from the configured endpoint") {
		t.Fatalf("read body = %q, not from the configured server", read.Output)
	}
	if hits.Load() == 0 {
		t.Fatal("the configured endpoint was never reached")
	}
}
