// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"net/http"
	"path/filepath"
	"testing"
	"time"
)

// TestMonitorConformance launches the monitor profile, confirms its read-only
// REST routes serve, then posts the control exit event and asserts the machine
// stops its owned listener and reaches the Done terminal state.
//
// It runs the wrapper an operator ships — agents/monitor/profile.yaml — through a
// temp copy, patching only the hard-coded bind address in rest.yaml so the
// listener takes a free loopback port. Nothing else is rebuilt.
//
// Traces srd008-monitor: the monitor serves read-only state routes while
// awaiting a control event, then stops the owned listener before terminating.
func TestMonitorConformance(t *testing.T) {
	RequireCoreRoot(t)
	addr := FreeAddr(t)

	profilePath := CopyShippedProfile(t, filepath.Join("agents", "monitor", "profile.yaml"), map[string]string{
		"127.0.0.1:0": addr,
	})

	server := Serve(t, ServeConfig{Profile: profilePath})
	server.WaitHealthy("http://"+addr+"/monitor/state", 15*time.Second)
	if status := server.Post("http://"+addr+"/monitor/control/exit", `{"reason":"conformance"}`); status != http.StatusAccepted {
		t.Fatalf("monitor control exit POST status = %d, want %d", status, http.StatusAccepted)
	}
	result := server.WaitExit(15 * time.Second)

	// srd008: clean terminal outcome with no error-status spans.
	result.RequireExit(t, 0)
	result.RootRequired(t)
	result.RequireNoErrorSpans(t)

	// srd008: launch, await, and listener-stop vocabulary is visible.
	result.RequireToolSpans(t, "launch_monitor_rest", "await_monitor_control", "stop_monitor_rest")

	// srd008: the machine reaches the Done terminal state.
	result.RequireTerminalState(t, "Done")
}
