// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"fmt"
	"net/http"
	"path/filepath"
	"testing"
	"time"
)

// TestBenchConformance launches the bench profile, waits for its web UI health
// route, and posts a shutdown action so the machine leaves the serve_ui block
// and reaches the Done terminal state.
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
	coreRoot := RequireCoreRoot(t)
	tmp := t.TempDir()
	addr := FreeAddr(t)
	benchDir := ProfilePath(filepath.Join("agents", "bench"))

	// builtin.yaml carries the serve_ui addr; bind it to a free port so the UI
	// host does not collide with a real bench server on :8080.
	builtinContent := rewriteFile(t, filepath.Join(benchDir, "builtin.yaml"), map[string]string{
		"addr: :8080": "addr: " + fmt.Sprintf("%q", addr),
	})
	writeEphemeral(t, tmp, "builtin.yaml", builtinContent)

	profilePath := writeEphemeral(t, tmp, "profile.yaml", fmt.Sprintf(`name: bench-conformance
machine: %q
tools:
  - %q
tool_config_dirs:
  - %q
tool_declarations:
  - %q
`, filepath.Join(benchDir, "machine.yaml"),
		filepath.Join(benchDir, "tools.yaml"),
		filepath.Join(coreRoot, "tools", "builtin", "bench"),
		filepath.Join(tmp, "builtin.yaml")))

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
