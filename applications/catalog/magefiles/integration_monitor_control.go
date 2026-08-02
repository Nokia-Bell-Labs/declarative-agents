// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"os"
	"path/filepath"
)

const monitorControlFixture = "testdata/integration/rel07-monitor-control"

type monitorControlEvidence struct {
	RuntimeStateReaderProfile string   `yaml:"runtime_state_reader_profile"`
	ControlProfile            string   `yaml:"control_profile"`
	MonitorStateRoutes        []string `yaml:"monitor_state_routes"`
	ControlExitRoute          string   `yaml:"control_exit_route"`
	// MonitorExitInjected records that the monitor server declares no static exit
	// route: lifecycle exit is provided by agent-core's injected /api/lifecycle/exit
	// (GH-1264), so the monitor server carries observability only.
	MonitorExitInjected bool `yaml:"monitor_exit_injected"`
	// MonitorAwaitRoute is the endpoint name the monitor lifecycle await filters on;
	// it must be the injected route name so the machine consumes the injected exit.
	MonitorAwaitRoute       string `yaml:"monitor_await_route"`
	MonitorExitSignal       string `yaml:"monitor_exit_signal"`
	ControlLifecycleSignal  string `yaml:"control_lifecycle_signal"`
	MonitorStopTransition   bool   `yaml:"monitor_stop_transition_declared"`
	HTTPHandlersEnqueueOnly bool   `yaml:"http_handlers_enqueue_only"`
	TargetOwner             string `yaml:"target_owner"`
}

// monitorAwaitConfig is the minimal shape of a tool declaration file needed to
// read the lifecycle await source a profile declares.
type monitorAwaitConfig struct {
	Tools []struct {
		Name   string `yaml:"name"`
		Config struct {
			Sources []struct {
				Server  string   `yaml:"server"`
				Routes  []string `yaml:"routes"`
				Signals []string `yaml:"signals"`
			} `yaml:"sources"`
		} `yaml:"config"`
	} `yaml:"tools"`
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
	profilesRoot, err := catalogOwnerRoot("catalog integration:monitorControl")
	if err != nil {
		return err
	}
	if err := requireProfilePaths(profilesRoot, "agents/runtime-state-reader/profile.yaml", "testdata/conformance/control/profile.yaml"); err != nil {
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
	runDir, err := os.MkdirTemp("", "catalog-monitor-control-*")
	if err != nil {
		return fmt.Errorf("create monitor-control run dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(runDir) }()
	if err := writeMonitorControlEvidence(runDir, evidence); err != nil {
		return err
	}
	if err := assertMonitorControlEvidence(runDir, expected); err != nil {
		return err
	}
	fmt.Println("integration:monitorControl PASS - observability routes and injected lifecycle exit wiring recorded")
	return nil
}

func collectMonitorControlEvidence(profilesRoot string) (monitorControlEvidence, error) {
	monitorREST, err := readMonitorControlREST(filepath.Join(profilesRoot, "agents", "runtime-state-reader", "rest.yaml"))
	if err != nil {
		return monitorControlEvidence{}, err
	}
	controlREST, err := readMonitorControlREST(filepath.Join(profilesRoot, "testdata", "conformance", "control", "rest.yaml"))
	if err != nil {
		return monitorControlEvidence{}, err
	}
	monitorMachine, err := readMonitorControlMachine(filepath.Join(profilesRoot, "agents", "runtime-state-reader", "machine.yaml"))
	if err != nil {
		return monitorControlEvidence{}, err
	}
	controlMachine, err := readMonitorControlMachine(filepath.Join(profilesRoot, "testdata", "conformance", "control", "machine.yaml"))
	if err != nil {
		return monitorControlEvidence{}, err
	}
	monitorAwait, err := readMonitorAwaitSource(
		filepath.Join(profilesRoot, "agents", "runtime-state-reader", "declarations.yaml"),
		"await_monitor_control", "monitor",
	)
	if err != nil {
		return monitorControlEvidence{}, err
	}
	controlExit, err := endpoint(controlREST, "agent_control", "exit")
	if err != nil {
		return monitorControlEvidence{}, err
	}
	evidence := monitorControlEvidence{
		RuntimeStateReaderProfile: "agents/runtime-state-reader/profile.yaml",
		ControlProfile:            "testdata/conformance/control/profile.yaml",
		MonitorStateRoutes:        monitorStateRoutes(monitorREST),
		ControlExitRoute:          controlExit.Path,
		MonitorExitInjected:       !monitorServerDeclaresExit(monitorREST),
		MonitorAwaitRoute:         monitorAwait.Route,
		MonitorExitSignal:         monitorAwait.Signal,
		ControlLifecycleSignal:    "AgentExited",
		MonitorStopTransition:     hasTransition(monitorMachine, "AwaitingControl", "ExitRequested", "Stopping", "stop_monitor_rest") && hasTransition(monitorMachine, "Stopping", "ServerStopped", "Done", ""),
		HTTPHandlersEnqueueOnly:   controlExit.Binding == "emit_signal" && hasTransition(controlMachine, "AwaitingControl", "ExitRequested", "Exiting", "exit_agent") && hasTransition(controlMachine, "Exiting", "AgentExited", "Succeeded", ""),
		TargetOwner:               "applications/catalog",
	}
	return evidence, nil
}

// monitorServerDeclaresExit reports whether the monitor server statically
// declares any lifecycle exit route. It must not: the monitor server carries
// observability only, and exit is provided by agent-core's injected endpoint.
func monitorServerDeclaresExit(rest monitorControlREST) bool {
	for _, ep := range rest.Rest.Servers["monitor"].Endpoints {
		if ep.Path == "/api/lifecycle/exit" || ep.Path == "/monitor/control/exit" {
			return true
		}
	}
	return false
}

type monitorAwaitSource struct {
	Route  string
	Signal string
}

// readMonitorAwaitSource returns the route filter and signal the named await
// word declares against the given server, proving the machine consumes the
// injected exit (route name 'exit') rather than a removed static route.
func readMonitorAwaitSource(path, word, server string) (monitorAwaitSource, error) {
	var decls monitorAwaitConfig
	if err := readIntegrationYAML(path, "tool declarations", &decls); err != nil {
		return monitorAwaitSource{}, err
	}
	for _, tool := range decls.Tools {
		if tool.Name != word {
			continue
		}
		for _, src := range tool.Config.Sources {
			if src.Server != server || len(src.Routes) == 0 || len(src.Signals) == 0 {
				continue
			}
			return monitorAwaitSource{Route: src.Routes[0], Signal: src.Signals[0]}, nil
		}
	}
	return monitorAwaitSource{}, fmt.Errorf("await source for %q on server %q not found in %s", word, server, path)
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
	if !got.MonitorStopTransition {
		return fmt.Errorf("monitor-control evidence missing declared monitor stop transition")
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
