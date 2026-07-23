// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// These cover the provisioning panel's initial mesh-view load (srd006 R1.5,
// GH-753): the panel reads it from the coordinator, not from the deployment
// API, which no browser may reach (srd003 R4.4). The chain is
// coordinator -> creator -> executor, mirroring the rollout read (GH-686); these
// pin every hop the mesh owns.

// stateFields are the flat fields the executor's state_response contract
// declares (srd006 deployment_api_contract). The wire shape is flat by
// necessity: machine_request response bodies map one selector per named field,
// so llm/params cannot be assembled from several source paths in one step.
var stateFields = []string{
	"rags", "llmInCluster", "llmExternalURL", "llmChatModel", "llmEmbedModel",
	"llmChatModels", "llmRouterModel", "llmTopology",
	"paramsNResults", "paramsChunkCap", "paramsRouterDefault",
}

// TestCoordinatorServesThePanelStateRead proves the panel's initial load has
// somewhere to land: a missing endpoint here would 404 rather than serve the
// mesh view.
func TestCoordinatorServesThePanelStateRead(t *testing.T) {
	state, ok := coordinatorIntentEndpoints(t)["state"]
	if !ok {
		t.Fatal("coordinator intent server declares no state endpoint; the panel's initial load would 404")
	}
	if state.Method != "GET" {
		t.Errorf("state method = %q, want GET", state.Method)
	}
	if state.Path != "/provisioning/api/state" {
		t.Errorf("state path = %q, want /provisioning/api/state (the path fetchMeshState calls)", state.Path)
	}
	if state.Binding != "machine_request" {
		t.Errorf("state binding = %q, want machine_request", state.Binding)
	}
}

// TestStateReadUsesItsOwnMachine proves a read cannot disturb a reconfiguration
// already in flight, and cannot enter the apply or rollout legs.
func TestStateReadUsesItsOwnMachine(t *testing.T) {
	endpoints := coordinatorIntentEndpoints(t)
	state := endpoints["state"]
	apply := endpoints["apply"]
	rollout := endpoints["rollout"]

	if state.MachineRequest.Machine == "" {
		t.Fatal("state endpoint names no machine")
	}
	if state.MachineRequest.Machine == apply.MachineRequest.Machine {
		t.Errorf("state and apply share machine %q; a read must not enter the apply legs", state.MachineRequest.Machine)
	}
	if state.MachineRequest.Machine == rollout.MachineRequest.Machine {
		t.Errorf("state and rollout share machine %q; each read owns its own machine", state.MachineRequest.Machine)
	}

	var machine intakeMachine
	readIntakeYAML(t, filepath.Join(agentDir(t, "coordinator"), state.MachineRequest.Machine), &machine)
	for _, tr := range machine.Transitions {
		if tr.Action == "request_ingest" || tr.Action == "request_rollout" {
			t.Errorf("the state machine drives %q; a read applies nothing", tr.Action)
		}
	}
}

// TestStateReadFailureIsNotASuccessfulEmptyView is the assertion that matters
// for the panel's first load. A broken read that returned 200 with an empty or
// default mesh view would render as a legitimate but topology-less mesh, which
// is worse than an error the panel can show.
func TestStateReadFailureIsNotASuccessfulEmptyView(t *testing.T) {
	state := coordinatorIntentEndpoints(t)["state"]
	states := state.MachineRequest.Response.TerminalStates
	if len(states) == 0 {
		t.Fatal("state maps no terminal states; a read failure and a successful read would share one status")
	}

	ok, present := states["StateRead"]
	if !present {
		t.Fatal("state does not map StateRead")
	}
	if ok.Status != 200 {
		t.Errorf("StateRead maps to %d, want 200", ok.Status)
	}

	failed, present := states["Failed"]
	if !present {
		t.Fatal("state does not map Failed; a read that cannot reach the creator would fall through")
	}
	if failed.Status < 400 {
		t.Errorf("Failed maps to %d, which the panel reads as success", failed.Status)
	}
}

// TestCreatorStateReadIsCoordinatorFacing proves the read exists on the creator
// and stays off the browser path, mirroring the rollout read's boundary
// (srd005 R5.4).
func TestCreatorStateReadIsCoordinatorFacing(t *testing.T) {
	var rest intakeRest
	readIntakeYAML(t, filepath.Join(agentDir(t, "creator"), "rest.yaml"), &rest)

	server, ok := rest.Rest.Servers["creator_instance"]
	if !ok {
		t.Fatal("creator_instance server not declared")
	}
	state, ok := server.Endpoints["state"]
	if !ok {
		t.Fatal("creator declares no state read; the coordinator has nothing to serve the panel's initial load from")
	}
	if state.Method != "GET" {
		t.Errorf("creator state method = %q, want GET", state.Method)
	}
	if state.MachineRequest.InitialSignal != "SeedState" {
		t.Errorf("creator state initial_signal = %q, want SeedState (the read_state leg alone)", state.MachineRequest.InitialSignal)
	}

	if strings.HasPrefix(state.Path, "/provisioning") {
		t.Errorf("creator state path %q is on the browser prefix; the creator is coordinator-facing only", state.Path)
	}

	states := state.MachineRequest.Response.TerminalStates
	if failed, present := states["Failed"]; !present || failed.Status < 400 {
		t.Error("creator state does not map Failed to an error status; a failed read would surface as a healthy empty view")
	}
}

// TestCreatorStateLegDrivesReadState proves the SeedState leg drives its own
// action rather than being silently folded into the health-verify leg, whose
// output shape (a phase and replica counts) is unrelated to the mesh view.
func TestCreatorStateLegDrivesReadState(t *testing.T) {
	var machine intakeMachine
	readIntakeYAML(t, filepath.Join(agentDir(t, "creator"), "request-machine.yaml"), &machine)

	var seeded bool
	for _, tr := range machine.Transitions {
		if tr.State == "AwaitingRequest" && tr.Signal == "SeedState" {
			seeded = true
			if tr.Action != "read_state" {
				t.Errorf("SeedState drives %q, want read_state", tr.Action)
			}
		}
	}
	if !seeded {
		t.Fatal("no AwaitingRequest + SeedState transition; the state-read leg does not exist")
	}
}

// TestStateFieldsSurviveTheCreatorHop proves the creator maps every flat field
// off the executor's response and re-serves it, rather than dropping fields the
// panel needs.
func TestStateFieldsSurviveTheCreatorHop(t *testing.T) {
	op := clientOperationNamed(t, "creator", "deployment_api", "read_state")
	for _, field := range stateFields {
		if got := op.Response.Output[field]; got != "$."+field {
			t.Errorf("read_state maps %s = %q, want $.%s; the executor serves it and the creator drops it", field, got, field)
		}
	}

	properties := wordOutputProperties(t, "creator", "request-declarations.yaml", "read_state")
	for _, field := range stateFields {
		if _, ok := properties[field]; !ok {
			t.Errorf("read_state output schema does not declare %s", field)
		}
	}

	var rest rolloutRest
	readIntakeYAML(t, filepath.Join(agentDir(t, "creator"), "rest.yaml"), &rest)
	reported, ok := rest.Rest.Servers["creator_instance"].Endpoints["state"].
		MachineRequest.Response.TerminalStates["StateReported"]
	if !ok {
		t.Fatal("creator state does not map StateReported")
	}
	for _, field := range stateFields {
		if got := reported.Body[field]; got != "$.mapped."+field {
			t.Errorf("creator state body %s = %q, want $.mapped.%s", field, got, field)
		}
	}
}

// TestStateFieldsSurviveTheCoordinatorHop proves the same for the hop the panel
// actually reads.
func TestStateFieldsSurviveTheCoordinatorHop(t *testing.T) {
	op := clientOperationNamed(t, "coordinator", "creator", "creator_state")
	for _, field := range stateFields {
		if got := op.Response.Output[field]; got != "$."+field {
			t.Errorf("creator_state maps %s = %q, want $.%s", field, got, field)
		}
	}

	properties := wordOutputProperties(t, "coordinator", "request-declarations.yaml", "read_creator_state")
	for _, field := range stateFields {
		if _, ok := properties[field]; !ok {
			t.Errorf("read_creator_state output schema does not declare %s", field)
		}
	}

	var rest rolloutRest
	readIntakeYAML(t, filepath.Join(agentDir(t, "coordinator"), "rest.yaml"), &rest)
	read, ok := rest.Rest.Servers["coordinator_intent"].Endpoints["state"].
		MachineRequest.Response.TerminalStates["StateRead"]
	if !ok {
		t.Fatal("coordinator state does not map StateRead")
	}
	for _, field := range stateFields {
		if got := read.Body[field]; got != "$.mapped."+field {
			t.Errorf("coordinator state body %s = %q, want $.mapped.%s", field, got, field)
		}
	}
}

// TestPanelWireStateInterfaceMatchesWhatIsServed proves provisioningApi.ts's
// MeshStateResponse -- the flat wire type fetchMeshState reshapes into the
// nested MeshView -- declares exactly the fields the coordinator's state read
// serves. A type promising a field no hop carries reads as a measurement in the
// UI; the fix is to serve it or narrow the interface, not fabricate a zero.
func TestPanelWireStateInterfaceMatchesWhatIsServed(t *testing.T) {
	meshRoot := filepath.Dir(filepath.Dir(agentDir(t, "coordinator")))
	source, err := os.ReadFile(filepath.Join(meshRoot, "ux", "app", "src", "provisioningApi.ts"))
	if err != nil {
		t.Fatalf("read provisioningApi.ts: %v", err)
	}
	block := regexp.MustCompile(`(?s)interface MeshStateResponse \{(.*?)\}`).FindSubmatch(source)
	if block == nil {
		t.Fatal("provisioningApi.ts declares no MeshStateResponse")
	}
	declared := map[string]bool{}
	for _, field := range regexp.MustCompile(`(?m)^\s*(\w+)(\??):`).FindAllSubmatch(block[1], -1) {
		declared[string(field[1])] = string(field[2]) == "?"
	}

	var rest rolloutRest
	readIntakeYAML(t, filepath.Join(agentDir(t, "coordinator"), "rest.yaml"), &rest)
	served := rest.Rest.Servers["coordinator_intent"].Endpoints["state"].
		MachineRequest.Response.TerminalStates["StateRead"].Body

	for field, optional := range declared {
		if _, ok := served[field]; !ok && !optional {
			t.Errorf("MeshStateResponse requires %s, which the state read never serves; serve it or narrow the interface", field)
		}
	}
	for field := range served {
		if field == "schema_version" {
			continue
		}
		if _, ok := declared[field]; !ok {
			t.Errorf("the state read serves %s, which MeshStateResponse does not declare", field)
		}
	}
}
