// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"net/http"
	"path/filepath"
	"testing"
	"time"
)

// TestRuntimeStateReaderConformance launches the runtime-state-reader profile,
// confirms its read-only monitor REST routes serve, then posts the control exit
// event and asserts the machine stops its owned listener and reaches Done.
//
// It runs the wrapper an operator ships — agents/runtime-state-reader/profile.yaml — through a
// temp copy, patching only the hard-coded bind address in rest.yaml so the
// listener takes a free loopback port. Nothing else is rebuilt.
//
// Traces srd008-runtime-state-reader: the adapter serves cached monitor state
// while awaiting a control event, then stops its listener before terminating.
func TestRuntimeStateReaderConformance(t *testing.T) {
	t.Parallel()
	RequireCoreRoot(t)
	addr := FreeAddr(t)

	profilePath := CopyShippedProfile(t, filepath.Join("agents", "runtime-state-reader", "profile.yaml"), map[string]string{
		"127.0.0.1:0": addr,
	})

	server := Serve(t, ServeConfig{Profile: profilePath})
	server.WaitHealthy("http://"+addr+"/monitor/state", 15*time.Second)
	if status := server.Post("http://"+addr+"/api/lifecycle/exit", `{"reason":"conformance"}`); status != http.StatusAccepted {
		t.Fatalf("lifecycle exit POST status = %d, want %d", status, http.StatusAccepted)
	}
	result := server.WaitExit(15 * time.Second)

	// srd008-runtime-state-reader: clean terminal outcome with no error-status spans.
	result.RequireExit(t, 0)
	result.RootRequired(t)
	result.RequireNoErrorSpans(t)

	// srd008-runtime-state-reader: accurate monitor lifecycle vocabulary remains visible.
	result.RequireToolSpans(t, "launch_monitor_rest", "await_monitor_control", "stop_monitor_rest")

	// srd008-runtime-state-reader: the machine reaches the Done terminal state.
	result.RequireTerminalState(t, "Done")
}
