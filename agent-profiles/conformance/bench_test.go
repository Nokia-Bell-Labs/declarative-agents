// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"net/http"
	"path/filepath"
	"testing"
	"time"
)

// TestBenchConformance launches the bench profile, waits for its web UI health
// route, and posts a shutdown action so the machine leaves the serve_ui block
// and reaches the Done terminal state.
//
// It runs the wrapper an operator ships — agents/bench/profile.yaml — through a
// temp copy, patching only the hard-coded serve_ui listen address in builtin.yaml
// so the UI host does not collide with a real bench server on :8080. The
// profile's /opt/agent-core tool_config_dir remaps onto the checkout via
// --core-root; nothing else is rebuilt.
//
// serve_ui is the bench equivalent of invoke_llm: it starts the HTTP server and
// blocks on the action channel until a human posts an action. The Serving -> Done
// path needs no evaluator launch, so this test drives only the shutdown action
// (the Serving -> Launching evaluator boundary is covered by #201's evaluator
// stub).
//
// Traces srd006-bench: serve_ui is the sole human input boundary, user actions
// route to machine signals, and shutdown reaches the Done terminal outcome.
func TestBenchConformance(t *testing.T) {
	RequireCoreRoot(t)
	addr := FreeAddr(t)

	profilePath := CopyShippedProfile(t, filepath.Join("agents", "bench", "profile.yaml"), map[string]string{
		"addr: :8080": `addr: "` + addr + `"`,
	})

	server := Serve(t, ServeConfig{Profile: profilePath})
	server.WaitHealthy("http://"+addr+"/api/v1/health", 15*time.Second)
	if status := server.Post("http://"+addr+"/api/v1/actions", `{"type":"shutdown"}`); status != http.StatusOK {
		t.Fatalf("shutdown action POST status = %d, want %d", status, http.StatusOK)
	}
	result := server.WaitExit(15 * time.Second)

	// srd006: clean terminal outcome with no error-status spans.
	result.RequireExit(t, 0)
	result.RootRequired(t)
	result.RequireNoErrorSpans(t)

	// srd006: serve_ui is the visible human-input boundary word.
	result.RequireToolSpans(t, "serve_ui")

	// srd006: the shutdown action reaches the Done terminal state.
	result.RequireTerminalState(t, "Done")
}
