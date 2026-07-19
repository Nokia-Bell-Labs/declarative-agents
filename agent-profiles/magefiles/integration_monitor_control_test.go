// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestCollectMonitorControlEvidenceRecordsRoutesAndLifecycleBoundary(t *testing.T) {
	root := t.TempDir()
	writeMonitorControlFixture(t, root, "exit_agent")

	evidence, err := collectMonitorControlEvidence(root)
	if err != nil {
		t.Fatalf("collectMonitorControlEvidence: %v", err)
	}
	if evidence.MonitorControlRoute != "/monitor/control/exit" {
		t.Fatalf("monitor control route = %q", evidence.MonitorControlRoute)
	}
	if evidence.ControlExitRoute != "/api/lifecycle/exit" {
		t.Fatalf("control exit route = %q", evidence.ControlExitRoute)
	}
	if evidence.ControlLifecycleSignal != "AgentExited" {
		t.Fatalf("control lifecycle signal = %q", evidence.ControlLifecycleSignal)
	}
	if !evidence.MonitorListenerCleanup {
		t.Fatalf("expected monitor listener cleanup evidence: %#v", evidence)
	}
	if !evidence.HTTPHandlersEnqueueOnly {
		t.Fatalf("expected enqueue-only lifecycle evidence: %#v", evidence)
	}
}

func TestAssertMonitorControlEvidenceRejectsMissingLifecycleRouting(t *testing.T) {
	runDir := t.TempDir()
	evidence := monitorControlEvidence{
		MonitorProfile:          "agents/monitor/profile.yaml",
		ControlProfile:          "testdata/conformance/control/profile.yaml",
		MonitorStateRoutes:      []string{"/monitor/state"},
		ControlExitRoute:        "/api/lifecycle/exit",
		MonitorControlRoute:     "/monitor/control/exit",
		MonitorExitSignal:       "ExitRequested",
		ControlLifecycleSignal:  "AgentExited",
		MonitorListenerCleanup:  true,
		HTTPHandlersEnqueueOnly: false,
		TargetOwner:             "agent-profiles",
	}
	if err := writeMonitorControlEvidence(runDir, evidence); err != nil {
		t.Fatalf("writeMonitorControlEvidence: %v", err)
	}

	err := assertMonitorControlEvidence(runDir, evidence)
	if err == nil {
		t.Fatal("expected missing lifecycle boundary error")
	}
	if !strings.Contains(err.Error(), "HTTP enqueue-only lifecycle boundary") {
		t.Fatalf("error = %q", err)
	}
}

func TestReadMonitorControlEvidenceParsesExpectedFixture(t *testing.T) {
	path := filepath.Join("..", "testdata", "integration", "rel07-monitor-control", "expected", "evidence.yaml")
	evidence, err := readMonitorControlEvidence(path)
	if err != nil {
		t.Fatalf("readMonitorControlEvidence: %v", err)
	}
	if evidence.TargetOwner != "agent-profiles" {
		t.Fatalf("target owner = %q", evidence.TargetOwner)
	}
	if len(evidence.MonitorStateRoutes) != 5 {
		t.Fatalf("monitor state routes = %#v", evidence.MonitorStateRoutes)
	}
}

func writeMonitorControlFixture(t *testing.T, root, controlAction string) {
	t.Helper()
	writeFile(t, filepath.Join(root, "agents", "monitor", "profile.yaml"), "name: monitor\n")
	writeFile(t, filepath.Join(root, "testdata", "conformance", "control", "profile.yaml"), "name: control\n")
	writeFile(t, filepath.Join(root, "agents", "monitor", "rest.yaml"), `rest:
  servers:
    monitor:
      endpoints:
        machine_spec: {path: /monitor/machine, binding: read_state}
        current_state: {path: /monitor/state, binding: read_state}
        tools: {path: /monitor/tools, binding: read_state}
        metrics: {path: /monitor/metrics, binding: read_state}
        recent_events: {path: /monitor/events, binding: read_state}
        control_exit:
          method: POST
          path: /monitor/control/exit
          binding: emit_signal
          signal: ExitRequested
`)
	writeFile(t, filepath.Join(root, "testdata", "conformance", "control", "rest.yaml"), `rest:
  servers:
    agent_control:
      endpoints:
        exit:
          method: POST
          path: /api/lifecycle/exit
          binding: emit_signal
          signal: ExitRequested
`)
	writeFile(t, filepath.Join(root, "agents", "monitor", "machine.yaml"), `transitions:
  - state: AwaitingControl
    signal: ExitRequested
    next: Stopping
    action: stop_monitor_rest
  - state: Stopping
    signal: ServerStopped
    next: Done
`)
	writeFile(t, filepath.Join(root, "testdata", "conformance", "control", "machine.yaml"), `transitions:
  - state: AwaitingControl
    signal: ExitRequested
    next: Exiting
    action: `+controlAction+`
  - state: Exiting
    signal: AgentExited
    next: Succeeded
`)
}
