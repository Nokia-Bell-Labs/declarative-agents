// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"gopkg.in/yaml.v3"
)

// These cover the coordinator as the panel's provisioning intake (GH-502,
// GH-680). The panel used to POST its apply at the executor, which the executor
// NetworkPolicy blocks because only creator-labelled pods may reach the apply
// surface. The intent now enters at the coordinator, which orchestrates the
// creator, which alone calls the executor (srd003 R4.1/R4.4, srd004 R1.5/R1.6,
// srd006 R4.1).

type intakeEndpoint struct {
	Method  string `yaml:"method"`
	Path    string `yaml:"path"`
	Binding string `yaml:"binding"`
	Request struct {
		BodySchema struct {
			Required   []string                  `yaml:"required"`
			Properties map[string]map[string]any `yaml:"properties"`
		} `yaml:"body_schema"`
	} `yaml:"request"`
	MachineRequest struct {
		InitialSignal string `yaml:"initial_signal"`
		Response      struct {
			TerminalStates  map[string]struct{ Status int } `yaml:"terminal_states"`
			TerminalSignals map[string]struct{ Status int } `yaml:"terminal_signals"`
		} `yaml:"response"`
	} `yaml:"machine_request"`
}

type intakeRest struct {
	Rest struct {
		Servers map[string]struct {
			Endpoints map[string]intakeEndpoint `yaml:"endpoints"`
		} `yaml:"servers"`
	} `yaml:"rest"`
}

type intakeMachine struct {
	Signals []struct {
		Name string `yaml:"name"`
	} `yaml:"signals"`
	TerminalStates []string `yaml:"terminal_states"`
	Transitions    []struct {
		State  string `yaml:"state"`
		Signal string `yaml:"signal"`
		Next   string `yaml:"next"`
		Action string `yaml:"action"`
	} `yaml:"transitions"`
}

// agentDir walks up to the mesh root and returns one agent's profile directory.
func agentDir(t *testing.T, agent string) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		candidate := filepath.Join(dir, "agents", agent)
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Skipf("agents/%s not found walking up from the test directory", agent)
		}
		dir = parent
	}
}

// envPlaceholder matches the ${NAME} and ${NAME:-default} forms the profiles use
// for deployment-supplied values.
var envPlaceholder = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(?::-([^}]*))?\}`)

// expandProfilePlaceholders substitutes each placeholder with its declared
// default, so a profile is parseable as plain YAML. A bare `${NAME}` inside a
// flow sequence -- `hosts: [${CREATOR_HOST:-127.0.0.1}]` -- is not valid YAML
// until it is expanded, which is why these tests cannot read the file directly.
// The runtime resolves from the environment; these tests assert on declared
// structure, so taking the defaults is what they want.
func expandProfilePlaceholders(data []byte) []byte {
	return envPlaceholder.ReplaceAllFunc(data, func(match []byte) []byte {
		groups := envPlaceholder.FindSubmatch(match)
		if len(groups) > 2 && groups[2] != nil {
			return groups[2]
		}
		return []byte("unset")
	})
}

func readIntakeYAML(t *testing.T, path string, out any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if err := yaml.Unmarshal(expandProfilePlaceholders(data), out); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
}

func coordinatorIntentEndpoints(t *testing.T) map[string]intakeEndpoint {
	t.Helper()
	var rest intakeRest
	readIntakeYAML(t, filepath.Join(agentDir(t, "coordinator"), "rest.yaml"), &rest)
	server, ok := rest.Rest.Servers["coordinator_intent"]
	if !ok {
		t.Fatal("coordinator_intent server not declared")
	}
	return server.Endpoints
}

// TestCoordinatorServesThePanelApplyPath proves the panel's apply is served by
// the coordinator at the path the SPA already calls, so only the ingress backend
// moves and the SPA's request paths stay put.
func TestCoordinatorServesThePanelApplyPath(t *testing.T) {
	apply, ok := coordinatorIntentEndpoints(t)["apply"]
	if !ok {
		t.Fatal("coordinator intent server declares no apply endpoint")
	}
	if apply.Method != "POST" {
		t.Errorf("apply method = %q, want POST", apply.Method)
	}
	if apply.Path != "/provisioning/api/apply" {
		t.Errorf("apply path = %q, want /provisioning/api/apply (the path ux/app/src/provisioningApi.ts calls)", apply.Path)
	}
	if apply.Binding != "machine_request" {
		t.Errorf("apply binding = %q, want machine_request", apply.Binding)
	}
}

// TestApplyAcceptsValuesOnlyIntent proves a pure values edit is expressible. The
// panel sends the whole desired mesh view and names no new source, so requiring
// collection and directory here would make a reconfiguration impossible to state
// (srd004 R1.6).
func TestApplyAcceptsValuesOnlyIntent(t *testing.T) {
	apply := coordinatorIntentEndpoints(t)["apply"]
	required := apply.Request.BodySchema.Required
	if len(required) != 1 || required[0] != "values" {
		t.Errorf("apply required fields = %v, want [values] only", required)
	}
	for _, field := range []string{"collection", "directory"} {
		if _, present := apply.Request.BodySchema.Properties[field]; present {
			t.Errorf("apply body declares %q; a values-only intent names no source", field)
		}
	}
}

// TestProvisionStillRequiresItsSource keeps the ingest-carrying intent strict. A
// request that names a source must still carry its collection and directory, so
// relaxing apply did not relax the other endpoint by accident.
func TestProvisionStillRequiresItsSource(t *testing.T) {
	provision, ok := coordinatorIntentEndpoints(t)["provision"]
	if !ok {
		t.Fatal("coordinator intent server declares no provision endpoint")
	}
	want := map[string]bool{"values": true, "collection": true, "directory": true}
	got := map[string]bool{}
	for _, field := range provision.Request.BodySchema.Required {
		got[field] = true
	}
	for field := range want {
		if !got[field] {
			t.Errorf("provision no longer requires %q", field)
		}
	}
}

// TestApplySeedsTheValuesOnlyLeg proves the branch is declarative. The endpoint
// picks the seed and the machine carries the fork, so which leg a run took is
// readable from the machine rather than hidden inside a word.
func TestApplySeedsTheValuesOnlyLeg(t *testing.T) {
	apply := coordinatorIntentEndpoints(t)["apply"]
	const seed = "SeedValues"
	if apply.MachineRequest.InitialSignal != seed {
		t.Fatalf("apply initial_signal = %q, want %q", apply.MachineRequest.InitialSignal, seed)
	}

	var machine intakeMachine
	readIntakeYAML(t, filepath.Join(agentDir(t, "coordinator"), "request-machine.yaml"), &machine)

	declared := false
	for _, s := range machine.Signals {
		if s.Name == seed {
			declared = true
		}
	}
	if !declared {
		t.Fatalf("machine does not declare the %s signal the apply endpoint seeds", seed)
	}

	var found bool
	for _, tr := range machine.Transitions {
		if tr.State != "AwaitingRequest" || tr.Signal != seed {
			continue
		}
		found = true
		if tr.Action != "request_rollout" {
			t.Errorf("%s action = %q, want request_rollout: a values-only apply has no directory to ingest", seed, tr.Action)
		}
		if tr.Next != "Reconfiguring" {
			t.Errorf("%s next = %q, want Reconfiguring", seed, tr.Next)
		}
	}
	if !found {
		t.Fatalf("no AwaitingRequest + %s transition; the values-only leg does not exist", seed)
	}

	// The ingest leg must survive unchanged.
	for _, tr := range machine.Transitions {
		if tr.State == "AwaitingRequest" && tr.Signal == "Seed" {
			if tr.Action != "request_ingest" || tr.Next != "Ingesting" {
				t.Errorf("the Seed leg changed: %s -> %s via %q", tr.Signal, tr.Next, tr.Action)
			}
			return
		}
	}
	t.Error("the Seed leg is gone; an intent naming a source can no longer ingest")
}

// TestApplyMapsOutcomesByTerminalState proves a rejected reconfiguration reaches
// the panel as a client error. The creator boundary words emit a fixed signal
// vocabulary, so signal keying could not separate a rejection from a failure
// (agent-core srd030 R4.6, GH-615).
func TestApplyMapsOutcomesByTerminalState(t *testing.T) {
	apply := coordinatorIntentEndpoints(t)["apply"]
	states := apply.MachineRequest.Response.TerminalStates
	if len(states) == 0 {
		t.Fatal("apply maps no terminal states; a rejection and a failure would collapse to one status")
	}

	want := map[string]int{"Reconfigured": 200, "Rejected": 422, "Failed": 500}
	for state, status := range want {
		mapping, ok := states[state]
		if !ok {
			t.Errorf("apply does not map terminal state %q", state)
			continue
		}
		if mapping.Status != status {
			t.Errorf("apply maps %q to %d, want %d", state, mapping.Status, status)
		}
	}

	var machine intakeMachine
	readIntakeYAML(t, filepath.Join(agentDir(t, "coordinator"), "request-machine.yaml"), &machine)
	terminal := map[string]bool{}
	for _, s := range machine.TerminalStates {
		terminal[s] = true
	}
	for state := range states {
		if !terminal[state] {
			t.Errorf("apply maps %q, which the machine does not declare terminal", state)
		}
	}
}
