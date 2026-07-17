// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"net/http"
	"path/filepath"
	"testing"
	"time"
)

// TestControlConformance launches the control profile, drives its REST
// lifecycle exit endpoint, and asserts the machine routes the enqueued signal
// through exit_agent to a Succeeded terminal state.
//
// It runs the wrapper an operator ships — agents/control/profile.yaml — through a
// temp copy, patching only the hard-coded bind address and port in rest.yaml so
// the listener takes a free loopback port. The profile's /opt/agent-core
// lifecycle tool_config_dir and exit-agent declaration remap onto the checkout
// via --core-root; nothing else is rebuilt.
//
// Traces srd010-control: HTTP handlers enqueue signals only, rest_await_event
// selects one control event, and exit_agent owns exit as visible lifecycle
// vocabulary.
func TestControlConformance(t *testing.T) {
	RequireCoreRoot(t)
	addr := FreeAddr(t)
	port := PortOf(t, addr)

	profilePath := CopyShippedProfile(t, filepath.Join("agents", "control", "profile.yaml"), map[string]string{
		"127.0.0.1:0": addr,
		"ports: [0]":  "ports: [" + port + "]",
	})

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
