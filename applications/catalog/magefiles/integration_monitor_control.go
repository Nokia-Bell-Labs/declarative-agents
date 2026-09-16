// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"fmt"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/pkg/profileaudit"
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
	coreRoot, err := resolveAgentCoreRoot(profilesRoot)
	if err != nil {
		return err
	}
	monitorAwait, err := readMonitorAwaitSource(
		filepath.Join(profilesRoot, "agents", "runtime-state-reader", "profile.yaml"),
		coreRoot, "await_monitor_control", "monitor",
	)
	if err != nil {
		return err
	}
	evidence, err := collectMonitorControlEvidence(profilesRoot, monitorAwait)
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

func collectMonitorControlEvidence(profilesRoot string, monitorAwait monitorAwaitSource) (monitorControlEvidence, error) {
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
//
// The word is read from the profile's loaded closure, not from its declaration
// file: since GH-2091 the reader instantiates it from a fragment, so the raw
// file carries an instantiate entry and no tool of that name. Loading is what
// makes imported and instantiated words visible (GH-2094).
func readMonitorAwaitSource(profilePath, coreRoot, word, server string) (monitorAwaitSource, error) {
	var found *monitorAwaitSource
	_, err := profileaudit.InspectWithOptions(profilePath, profileaudit.Options{
		CoreRoot: coreRoot,
		OnMachine: func(reached profileaudit.ReachedMachine) error {
			if reached.RequestScoped || found != nil {
				return nil
			}
			for _, tool := range reached.Selected {
				if tool.Name != word {
					continue
				}
				if source, ok := awaitSourceFor(tool.Config, server); ok {
					found = &source
				}
			}
			return nil
		},
	})
	if err != nil {
		return monitorAwaitSource{}, fmt.Errorf("load %s: %w", profilePath, err)
	}
	if found == nil {
		return monitorAwaitSource{}, fmt.Errorf(
			"await source for %q on server %q not found among the tools %s selects", word, server, profilePath)
	}
	return *found, nil
}

// awaitSourceFor reads the first configured source on server from a
// rest_await_event word's config: sources[].{server, routes, signals}.
func awaitSourceFor(config map[string]interface{}, server string) (monitorAwaitSource, bool) {
	sources, _ := config["sources"].([]interface{})
	for _, raw := range sources {
		source, _ := raw.(map[string]interface{})
		if fmt.Sprint(source["server"]) != server {
			continue
		}
		routes := stringsOf(source["routes"])
		signals := stringsOf(source["signals"])
		if len(routes) == 0 || len(signals) == 0 {
			continue
		}
		return monitorAwaitSource{Route: routes[0], Signal: signals[0]}, true
	}
	return monitorAwaitSource{}, false
}

func stringsOf(value interface{}) []string {
	items, _ := value.([]interface{})
	out := make([]string, 0, len(items))
	for _, item := range items {
		if text, ok := item.(string); ok {
			out = append(out, text)
		}
	}
	return out
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
