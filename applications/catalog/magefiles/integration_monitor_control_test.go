// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCollectMonitorControlEvidenceRecordsRoutesAndLifecycleBoundary(t *testing.T) {
	root := t.TempDir()
	writeMonitorControlFixture(t, root, "exit_agent")

	evidence, err := collectMonitorControlEvidence(root, monitorAwaitSource{Route: "exit", Signal: "ExitRequested"}, fixtureMonitorControlMachine)
	if err != nil {
		t.Fatalf("collectMonitorControlEvidence: %v", err)
	}
	if !evidence.MonitorExitInjected {
		t.Fatalf("expected monitor server to declare no static exit route: %#v", evidence)
	}
	if evidence.MonitorAwaitRoute != "exit" {
		t.Fatalf("monitor await route = %q", evidence.MonitorAwaitRoute)
	}
	if evidence.MonitorExitSignal != "ExitRequested" {
		t.Fatalf("monitor exit signal = %q", evidence.MonitorExitSignal)
	}
	if evidence.ControlExitRoute != "/api/lifecycle/exit" {
		t.Fatalf("control exit route = %q", evidence.ControlExitRoute)
	}
	if evidence.ControlLifecycleSignal != "AgentExited" {
		t.Fatalf("control lifecycle signal = %q", evidence.ControlLifecycleSignal)
	}
	if !evidence.MonitorStopTransition {
		t.Fatalf("expected declared monitor stop transition evidence: %#v", evidence)
	}
	if !evidence.HTTPHandlersEnqueueOnly {
		t.Fatalf("expected enqueue-only lifecycle evidence: %#v", evidence)
	}
}

func TestAssertMonitorControlEvidenceRejectsMissingLifecycleRouting(t *testing.T) {
	runDir := t.TempDir()
	evidence := monitorControlEvidence{
		RuntimeStateReaderProfile: "agents/runtime-state-reader/profile.yaml",
		ControlProfile:            "testdata/conformance/control/profile.yaml",
		MonitorStateRoutes:        []string{"/monitor/state"},
		ControlExitRoute:          "/api/lifecycle/exit",
		MonitorExitInjected:       true,
		MonitorAwaitRoute:         "exit",
		MonitorExitSignal:         "ExitRequested",
		ControlLifecycleSignal:    "AgentExited",
		MonitorStopTransition:     true,
		HTTPHandlersEnqueueOnly:   false,
		TargetOwner:               "applications/catalog",
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
	if evidence.TargetOwner != "applications/catalog" {
		t.Fatalf("target owner = %q", evidence.TargetOwner)
	}
	if len(evidence.MonitorStateRoutes) != 5 {
		t.Fatalf("monitor state routes = %#v", evidence.MonitorStateRoutes)
	}
}

// fixtureMonitorControlMachine reads the hand-written machine.yaml beside a
// synthetic fixture profile; the shipped profiles are read through the loader.
func fixtureMonitorControlMachine(profilesRoot, profile string) (monitorControlMachine, error) {
	var machine monitorControlMachine
	path := filepath.Join(profilesRoot, filepath.Dir(filepath.FromSlash(profile)), "machine.yaml")
	if err := readIntegrationYAML(path, "machine", &machine); err != nil {
		return monitorControlMachine{}, err
	}
	return machine, nil
}

// TestMonitorControlEvidenceLoadsTheShippedMachines is GH-2132: the shipped
// runtime-state-reader machine is a template instance, so evidence gathered
// from the loaded machines must still see the stop and lifecycle routes.
func TestMonitorControlEvidenceLoadsTheShippedMachines(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	coreRoot, err := resolveAgentCoreRoot(root)
	if err != nil {
		t.Skipf("agent-core checkout not resolvable beside the catalog: %v", err)
	}
	await, err := readMonitorAwaitSource(
		filepath.Join(root, "agents", "runtime-state-reader", "profile.yaml"),
		coreRoot, "await_monitor_control", "monitor",
	)
	if err != nil {
		t.Fatal(err)
	}

	evidence, err := collectMonitorControlEvidence(root, await, loadedMonitorControlMachine(coreRoot))

	if err != nil {
		t.Fatalf("collectMonitorControlEvidence: %v", err)
	}
	if !evidence.MonitorStopTransition {
		t.Fatalf("monitor stop transition not found in the loaded runtime-state-reader machine: %#v", evidence)
	}
	if !evidence.HTTPHandlersEnqueueOnly {
		t.Fatalf("lifecycle routing not found in the loaded control machine: %#v", evidence)
	}
	expected, err := readMonitorControlEvidence(filepath.Join(root, monitorControlFixture, "expected", "evidence.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(evidence, expected) {
		t.Fatalf("evidence = %#v\nwant %#v", evidence, expected)
	}
}

func writeMonitorControlFixture(t *testing.T, root, controlAction string) {
	t.Helper()
	writeFile(t, filepath.Join(root, "agents", "runtime-state-reader", "profile.yaml"), "name: runtime-state-reader\n")
	writeFile(t, filepath.Join(root, "testdata", "conformance", "control", "profile.yaml"), "name: control\n")
	// The monitor server carries observability routes only; lifecycle exit is
	// agent-core-injected, so no static exit route is declared here.
	writeFile(t, filepath.Join(root, "agents", "runtime-state-reader", "rest.yaml"), `rest:
  servers:
    monitor:
      endpoints:
        machine_spec: {path: /monitor/machine, binding: read_state}
        current_state: {path: /monitor/state, binding: read_state}
        tools: {path: /monitor/tools, binding: read_state}
        metrics: {path: /monitor/metrics, binding: read_state}
        recent_events: {path: /monitor/events, binding: read_state}
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
	writeFile(t, filepath.Join(root, "agents", "runtime-state-reader", "machine.yaml"), `transitions:
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

// TestReadMonitorAwaitSourceSeesTheInstantiatedWord is GH-2094 as a test: the
// shipped runtime-state-reader instantiates await_monitor_control from the
// monitor-control fragment, so a reader of the raw declarations file finds no
// such word, and only the loaded closure does.
func TestReadMonitorAwaitSourceSeesTheInstantiatedWord(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	coreRoot, err := resolveAgentCoreRoot(root)
	if err != nil {
		t.Skipf("agent-core checkout not resolvable beside the catalog: %v", err)
	}

	source, err := readMonitorAwaitSource(
		filepath.Join(root, "agents", "runtime-state-reader", "profile.yaml"),
		coreRoot, "await_monitor_control", "monitor",
	)
	if err != nil {
		t.Fatalf("readMonitorAwaitSource: %v", err)
	}
	if source.Route != "exit" || source.Signal != "ExitRequested" {
		t.Fatalf("await source = %#v, want route exit and signal ExitRequested", source)
	}
}
