// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"net/http"
	"path/filepath"
	"testing"
	"time"
)

func TestCollectorSpoolModeConformance(t *testing.T) {
	RequireCoreRoot(t)
	controlAddr := FreeAddr(t)
	monitorAddr := FreeAddr(t)
	receiverAddr := FreeAddr(t)

	profilePath := CopyShippedProfile(t, filepath.Join("agents", "collector", "profile.yaml"), map[string]string{
		"127.0.0.1:${COLLECTOR_CONTROL_PORT:-18191}": controlAddr,
		"127.0.0.1:${COLLECTOR_MONITOR_PORT:-18192}":  monitorAddr,
		"${COLLECTOR_BIND_HOST:-127.0.0.1}:${COLLECTOR_CONTROL_PORT:-18191}": controlAddr,
		"${COLLECTOR_BIND_HOST:-127.0.0.1}:${COLLECTOR_MONITOR_PORT:-18192}": monitorAddr,
		"0.0.0.0:4317": receiverAddr,
	})

	server := Serve(t, ServeConfig{Profile: profilePath})
	server.WaitHealthy("http://"+controlAddr+"/api/lifecycle/health", 15*time.Second)

	if status := server.Post("http://"+controlAddr+"/api/lifecycle/exit", `{"reason":"conformance"}`); status != http.StatusAccepted {
		t.Fatalf("control exit POST status = %d, want %d", status, http.StatusAccepted)
	}
	result := server.WaitExit(35 * time.Second)

	result.RequireExit(t, 0)
	result.RootRequired(t)
	result.RequireNoErrorSpans(t)
	result.RequireToolSpans(t,
		"otlp_receiver_launch",
		"launch_collector_control",
		"launch_collector_monitor",
		"await_spans",
		"await_collector_control",
		"exit_agent",
		"otlp_receiver_stop",
		"stop_collector_monitor",
		"stop_collector_control",
	)
	result.RequireTerminalState(t, "Done")
}
