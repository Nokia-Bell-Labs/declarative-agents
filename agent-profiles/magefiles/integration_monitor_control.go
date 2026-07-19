// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"os"
	"path/filepath"
)

const monitorControlFixture = "testdata/integration/rel07-monitor-control"

type monitorControlEvidence struct {
	MonitorProfile          string   `yaml:"monitor_profile"`
	ControlProfile          string   `yaml:"control_profile"`
	MonitorStateRoutes      []string `yaml:"monitor_state_routes"`
	ControlExitRoute        string   `yaml:"control_exit_route"`
	MonitorControlRoute     string   `yaml:"monitor_control_route"`
	MonitorExitSignal       string   `yaml:"monitor_exit_signal"`
	ControlLifecycleSignal  string   `yaml:"control_lifecycle_signal"`
	MonitorListenerCleanup  bool     `yaml:"monitor_listener_cleanup"`
	HTTPHandlersEnqueueOnly bool     `yaml:"http_handlers_enqueue_only"`
	TargetOwner             string   `yaml:"target_owner"`
}

type monitorControlMachine struct {
	Transitions []struct {
		State  string `yaml:"state"`
		Signal string `yaml:"signal"`
		Next   string `yaml:"next"`
		Action string `yaml:"action"`
	} `yaml:"transitions"`
}

type monitorControlREST struct {
	Rest struct {
		Servers map[string]struct {
			Endpoints map[string]struct {
				Method  string `yaml:"method"`
				Path    string `yaml:"path"`
				Binding string `yaml:"binding"`
				Signal  string `yaml:"signal"`
			} `yaml:"endpoints"`
		} `yaml:"servers"`
	} `yaml:"rest"`
}

// MonitorControl proves monitor state routes and REST lifecycle signal routing.
func (Integration) MonitorControl() error {
	profilesRoot, err := os.Getwd()
	if err != nil {
		return err
	}
	if err := requireProfilePaths(profilesRoot, "agents/monitor/profile.yaml", "testdata/conformance/control/profile.yaml"); err != nil {
		return err
	}
	evidence, err := collectMonitorControlEvidence(profilesRoot)
	if err != nil {
		return err
	}
	expected, err := readMonitorControlEvidence(filepath.Join(profilesRoot, monitorControlFixture, "expected", "evidence.yaml"))
	if err != nil {
		return err
	}
	runDir, err := os.MkdirTemp("", "agent-profiles-monitor-control-*")
	if err != nil {
		return fmt.Errorf("create monitor-control run dir: %w", err)
	}
	defer os.RemoveAll(runDir)
	if err := writeMonitorControlEvidence(runDir, evidence); err != nil {
		return err
	}
	if err := assertMonitorControlEvidence(runDir, expected); err != nil {
		return err
	}
	fmt.Println("integration:monitorControl PASS - monitor routes and control lifecycle boundary recorded")
	return nil
}

func collectMonitorControlEvidence(profilesRoot string) (monitorControlEvidence, error) {
	monitorREST, err := readMonitorControlREST(filepath.Join(profilesRoot, "agents", "monitor", "rest.yaml"))
	if err != nil {
		return monitorControlEvidence{}, err
	}
	controlREST, err := readMonitorControlREST(filepath.Join(profilesRoot, "testdata", "conformance", "control", "rest.yaml"))
	if err != nil {
		return monitorControlEvidence{}, err
	}
	monitorMachine, err := readMonitorControlMachine(filepath.Join(profilesRoot, "agents", "monitor", "machine.yaml"))
	if err != nil {
		return monitorControlEvidence{}, err
	}
	controlMachine, err := readMonitorControlMachine(filepath.Join(profilesRoot, "testdata", "conformance", "control", "machine.yaml"))
	if err != nil {
		return monitorControlEvidence{}, err
	}
	monitorControl, err := endpoint(monitorREST, "monitor", "control_exit")
	if err != nil {
		return monitorControlEvidence{}, err
	}
	controlExit, err := endpoint(controlREST, "agent_control", "exit")
	if err != nil {
		return monitorControlEvidence{}, err
	}
	evidence := monitorControlEvidence{
		MonitorProfile:          "agents/monitor/profile.yaml",
		ControlProfile:          "testdata/conformance/control/profile.yaml",
		MonitorStateRoutes:      monitorStateRoutes(monitorREST),
		ControlExitRoute:        controlExit.Path,
		MonitorControlRoute:     monitorControl.Path,
		MonitorExitSignal:       monitorControl.Signal,
		ControlLifecycleSignal:  "AgentExited",
		MonitorListenerCleanup:  hasTransition(monitorMachine, "AwaitingControl", "ExitRequested", "Stopping", "stop_monitor_rest") && hasTransition(monitorMachine, "Stopping", "ServerStopped", "Done", ""),
		HTTPHandlersEnqueueOnly: monitorControl.Binding == "emit_signal" && controlExit.Binding == "emit_signal" && hasTransition(controlMachine, "AwaitingControl", "ExitRequested", "Exiting", "exit_agent") && hasTransition(controlMachine, "Exiting", "AgentExited", "Succeeded", ""),
		TargetOwner:             "agent-profiles",
	}
	return evidence, nil
}

func readMonitorControlEvidence(path string) (monitorControlEvidence, error) {
	var evidence monitorControlEvidence
	if err := readIntegrationYAML(path, "monitor-control evidence", &evidence); err != nil {
		return monitorControlEvidence{}, err
	}
	return evidence, nil
}

func writeMonitorControlEvidence(runDir string, evidence monitorControlEvidence) error {
	return writeIntegrationYAML(filepath.Join(runDir, "evidence.yaml"), "monitor-control evidence", evidence)
}

func assertMonitorControlEvidence(runDir string, want monitorControlEvidence) error {
	got, err := readMonitorControlEvidence(filepath.Join(runDir, "evidence.yaml"))
	if err != nil {
		return err
	}
	if fmt.Sprintf("%#v", got) != fmt.Sprintf("%#v", want) {
		return fmt.Errorf("monitor-control evidence = %#v, want %#v", got, want)
	}
	if !got.MonitorListenerCleanup {
		return fmt.Errorf("monitor-control evidence missing monitor listener cleanup")
	}
	if !got.HTTPHandlersEnqueueOnly {
		return fmt.Errorf("monitor-control evidence missing HTTP enqueue-only lifecycle boundary")
	}
	return nil
}

func readMonitorControlREST(path string) (monitorControlREST, error) {
	var rest monitorControlREST
	if err := readIntegrationYAML(path, "REST config", &rest); err != nil {
		return monitorControlREST{}, err
	}
	return rest, nil
}

func readMonitorControlMachine(path string) (monitorControlMachine, error) {
	var machine monitorControlMachine
	if err := readIntegrationYAML(path, "machine", &machine); err != nil {
		return monitorControlMachine{}, err
	}
	return machine, nil
}

func endpoint(rest monitorControlREST, server, route string) (struct {
	Method  string `yaml:"method"`
	Path    string `yaml:"path"`
	Binding string `yaml:"binding"`
	Signal  string `yaml:"signal"`
}, error) {
	def, ok := rest.Rest.Servers[server]
	if !ok {
		return struct {
			Method  string `yaml:"method"`
			Path    string `yaml:"path"`
			Binding string `yaml:"binding"`
			Signal  string `yaml:"signal"`
		}{}, fmt.Errorf("server %q not found", server)
	}
	ep, ok := def.Endpoints[route]
	if !ok {
		return struct {
			Method  string `yaml:"method"`
			Path    string `yaml:"path"`
			Binding string `yaml:"binding"`
			Signal  string `yaml:"signal"`
		}{}, fmt.Errorf("route %q not found on server %q", route, server)
	}
	return ep, nil
}

func monitorStateRoutes(rest monitorControlREST) []string {
	def := rest.Rest.Servers["monitor"]
	var routes []string
	for _, name := range []string{"machine_spec", "current_state", "tools", "metrics", "recent_events"} {
		routes = append(routes, def.Endpoints[name].Path)
	}
	return routes
}

func hasTransition(machine monitorControlMachine, state, signal, next, action string) bool {
	for _, transition := range machine.Transitions {
		if transition.State == state && transition.Signal == signal && transition.Next == next && (action == "" || transition.Action == action) {
			return true
		}
	}
	return false
}
