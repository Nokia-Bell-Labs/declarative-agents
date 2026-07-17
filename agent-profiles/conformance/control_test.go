// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"fmt"
	"net/http"
	"path/filepath"
	"testing"
	"time"
)

// TestControlConformance launches the control profile, drives its REST
// lifecycle exit endpoint, and asserts the machine routes the enqueued signal
// through exit_agent to a Succeeded terminal state.
//
// Traces srd010-control: HTTP handlers enqueue signals only, rest_await_event
// selects one control event, and exit_agent owns exit as visible lifecycle
// vocabulary.
func TestControlConformance(t *testing.T) {
	coreRoot := RequireCoreRoot(t)
	tmp := t.TempDir()
	addr := FreeAddr(t)
	port := PortOf(t, addr)
	ctrlDir := ProfilePath(filepath.Join("agents", "control"))

	restContent := rewriteFile(t, filepath.Join(ctrlDir, "rest.yaml"), map[string]string{
		"127.0.0.1:0": addr,
		"ports: [0]":  "ports: [" + port + "]",
	})
	restPath := writeEphemeral(t, tmp, "rest.yaml", restContent)
	profilePath := writeEphemeral(t, tmp, "profile.yaml", fmt.Sprintf(`name: control-conformance
machine: %q
tools:
  - %q
tool_config_dirs:
  - %q
tool_declarations:
  - %q
  - %q
rest_definitions:
  - %q
`, filepath.Join(ctrlDir, "machine.yaml"),
		filepath.Join(ctrlDir, "tools.yaml"),
		filepath.Join(coreRoot, "tools", "builtin", "lifecycle"),
		filepath.Join(coreRoot, "tools", "builtin", "lifecycle", "exit-agent.yaml"),
		filepath.Join(ctrlDir, "declarations.yaml"),
		restPath))

	server := Serve(t, ServeConfig{Profile: profilePath})
	server.WaitHealthy("http://"+addr+"/health", 15*time.Second)
	if status := server.Post("http://"+addr+"/api/lifecycle/exit", `{"reason":"conformance","status":"success"}`); status != http.StatusAccepted {
		t.Fatalf("lifecycle exit POST status = %d, want %d", status, http.StatusAccepted)
	}
	result := server.WaitExit(15 * time.Second)

	// srd010: clean terminal outcome with no error-status spans.
	result.RequireExit(t, 0)
	result.RootRequired(t)
	result.RequireNoErrorSpans(t)

	// srd010: the enqueue -> await -> exit_agent lifecycle vocabulary is visible.
	result.RequireToolSpans(t, "launch_agent_control", "await_agent_control", "exit_agent")

	// srd010: the machine reaches the Succeeded terminal state.
	result.RequireTerminalState(t, "Succeeded")
}
