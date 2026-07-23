// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
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
		Machine       string `yaml:"machine"`
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
		// request_rollout_values, not request_rollout: with no ingest step ahead
		// of it this leg has no carried intent, and request_rollout's selector
		// ($.carried.values) found nothing, so every panel apply died in request
		// rendering with `body field "values" is required`. This assertion pinned
		// that break in place until GH-755 (the word reads $.values instead).
		if tr.Action != "request_rollout_values" {
			t.Errorf("%s action = %q, want request_rollout_values: a values-only apply reads the desired document off the seed, not off a carried intent", seed, tr.Action)
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

// TestEachRolloutLegReadsItsOwnProvenance pins the defect GH-755 fixed: the two
// rollout words differ only in where the desired values document comes from, and
// reading the wrong one fails in request rendering rather than at config load, so
// nothing catches it until a request actually runs. The values leg reads the seed
// ($.values); the ingest leg reads what creator_ingest carried forward
// ($.carried.values). Swapping them silently breaks one intake apiece.
func TestEachRolloutLegReadsItsOwnProvenance(t *testing.T) {
	for _, tc := range []struct{ operation, selector, why string }{
		{"creator_rollout_values", "$.values", "the values-only apply has no ingest step ahead of it, so nothing populated $.carried"},
		{"creator_rollout", "$.carried.values", "the ingest leg's document arrives via creator_ingest's carry_forward"},
	} {
		t.Run(tc.operation, func(t *testing.T) {
			op := clientOperationNamed(t, "coordinator", "creator", tc.operation)
			if got := op.Response.Output; len(got) == 0 {
				t.Errorf("%s maps no response output", tc.operation)
			}
			var rest struct {
				Rest struct {
					Clients map[string]struct {
						Operations map[string]struct {
							Params struct {
								InputMapping map[string]string `yaml:"input_mapping"`
							} `yaml:"params"`
						} `yaml:"operations"`
					} `yaml:"clients"`
				} `yaml:"rest"`
			}
			readIntakeYAML(t, filepath.Join(agentDir(t, "coordinator"), "rest.yaml"), &rest)
			got := rest.Rest.Clients["creator"].Operations[tc.operation].Params.InputMapping["values"]
			if got != tc.selector {
				t.Errorf("%s maps values = %q, want %q: %s", tc.operation, got, tc.selector, tc.why)
			}
		})
	}
}

// TestCreatorInstanceRequiresContent proves a values-less operation is refused as
// a declared client error. Every operation the creator realizes goes through
// apply_instance, whose body is {schema_version, content}; while content was
// optional the request died inside rendering and surfaced as an opaque 500, which
// is what broke mage integration:controlPlane (GH-755). The corpus-ingest child
// run srd005 R3.1 specifies needs its own leg, not a values apply missing a field.
func TestCreatorInstanceRequiresContent(t *testing.T) {
	var rest intakeRest
	readIntakeYAML(t, filepath.Join(agentDir(t, "creator"), "rest.yaml"), &rest)
	instance, ok := rest.Rest.Servers["creator_instance"].Endpoints["instance"]
	if !ok {
		t.Fatal("creator declares no instance endpoint")
	}
	required := map[string]bool{}
	for _, field := range instance.Request.BodySchema.Required {
		required[field] = true
	}
	if !required["content"] {
		t.Errorf("instance required fields = %v, want content among them; without it a values-less operation fails in request rendering as a 500 instead of a 400",
			instance.Request.BodySchema.Required)
	}
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

// The panel polls rollout progress from the coordinator, not from the deployment
// API, which no browser may reach (srd003 R4.4, srd004 R1.3). The chain is
// coordinator -> creator -> executor; these pin both hops the mesh owns (GH-684).

// TestCoordinatorServesThePanelRolloutPoll proves the poll has somewhere to land.
// After GH-681 the /provisioning prefix routes to the coordinator, so a missing
// endpoint here would 404 the panel's poll rather than 403 it.
func TestCoordinatorServesThePanelRolloutPoll(t *testing.T) {
	rollout, ok := coordinatorIntentEndpoints(t)["rollout"]
	if !ok {
		t.Fatal("coordinator intent server declares no rollout endpoint; the panel's poll would 404")
	}
	if rollout.Method != "GET" {
		t.Errorf("rollout method = %q, want GET", rollout.Method)
	}
	if rollout.Path != "/provisioning/api/rollout" {
		t.Errorf("rollout path = %q, want /provisioning/api/rollout (the path useRollout polls)", rollout.Path)
	}
}

// TestRolloutPollUsesItsOwnMachine proves a poll cannot disturb a reconfiguration
// already in flight. Sharing the provisioning machine would put a read on the
// same legs as an apply.
func TestRolloutPollUsesItsOwnMachine(t *testing.T) {
	endpoints := coordinatorIntentEndpoints(t)
	rollout := endpoints["rollout"]
	apply := endpoints["apply"]

	if rollout.MachineRequest.Machine == "" {
		t.Fatal("rollout endpoint names no machine")
	}
	if rollout.MachineRequest.Machine == apply.MachineRequest.Machine {
		t.Errorf("rollout and apply share machine %q; a poll must not enter the apply legs", rollout.MachineRequest.Machine)
	}

	var machine intakeMachine
	readIntakeYAML(t, filepath.Join(agentDir(t, "coordinator"), rollout.MachineRequest.Machine), &machine)
	for _, tr := range machine.Transitions {
		if tr.Action == "request_ingest" || tr.Action == "request_rollout" {
			t.Errorf("the poll machine drives %q; a read applies nothing", tr.Action)
		}
	}
}

// TestRolloutReadFailureIsNotASuccessfulUnknown is the assertion that matters for
// a poller. If a broken read returned 200 with an unknown phase, the panel would
// show "progressing" forever and the operator would never learn the read failed.
func TestRolloutReadFailureIsNotASuccessfulUnknown(t *testing.T) {
	rollout := coordinatorIntentEndpoints(t)["rollout"]
	states := rollout.MachineRequest.Response.TerminalStates
	if len(states) == 0 {
		t.Fatal("rollout maps no terminal states; a read failure and a successful read would share one status")
	}

	ok, present := states["StatusRead"]
	if !present {
		t.Fatal("rollout does not map StatusRead")
	}
	if ok.Status != 200 {
		t.Errorf("StatusRead maps to %d, want 200", ok.Status)
	}

	failed, present := states["Failed"]
	if !present {
		t.Fatal("rollout does not map Failed; a read that cannot reach the creator would fall through")
	}
	if failed.Status < 400 {
		t.Errorf("Failed maps to %d, which the panel reads as success", failed.Status)
	}
}

// TestCreatorRolloutReadIsCoordinatorFacing proves the read exists on the creator
// and stays off the browser path. The creator exposes no browser-facing surface
// (srd005 R5.4) -- it is the only pod the executor admits to the apply surface, so
// widening its reachability would widen the path to apply.
func TestCreatorRolloutReadIsCoordinatorFacing(t *testing.T) {
	var rest intakeRest
	readIntakeYAML(t, filepath.Join(agentDir(t, "creator"), "rest.yaml"), &rest)

	server, ok := rest.Rest.Servers["creator_instance"]
	if !ok {
		t.Fatal("creator_instance server not declared")
	}
	rollout, ok := server.Endpoints["rollout"]
	if !ok {
		t.Fatal("creator declares no rollout read; the coordinator has nothing to poll")
	}
	if rollout.Method != "GET" {
		t.Errorf("creator rollout method = %q, want GET", rollout.Method)
	}
	if rollout.MachineRequest.InitialSignal != "SeedHealth" {
		t.Errorf("creator rollout initial_signal = %q, want SeedHealth (the verify leg alone)", rollout.MachineRequest.InitialSignal)
	}

	// The read must not be served on a browser-reachable path prefix. Only the
	// coordinator's intent server carries /provisioning.
	if strings.HasPrefix(rollout.Path, "/provisioning") {
		t.Errorf("creator rollout path %q is on the browser prefix; the creator is coordinator-facing only", rollout.Path)
	}

	states := rollout.MachineRequest.Response.TerminalStates
	if failed, present := states["Failed"]; !present || failed.Status < 400 {
		t.Error("creator rollout does not map Failed to an error status; a failed read would surface as a healthy phase")
	}
}

// TestCreatorHealthLegIsReusedNotDuplicated proves the read runs the existing
// verify leg. A second way to read the same thing would drift from the first.
func TestCreatorHealthLegIsReusedNotDuplicated(t *testing.T) {
	var machine intakeMachine
	readIntakeYAML(t, filepath.Join(agentDir(t, "creator"), "request-machine.yaml"), &machine)

	var seeded bool
	for _, tr := range machine.Transitions {
		if tr.State == "AwaitingRequest" && tr.Signal == "SeedHealth" {
			seeded = true
			if tr.Action != "verify_health" {
				t.Errorf("SeedHealth drives %q, want verify_health", tr.Action)
			}
			if tr.Next != "Verifying" {
				t.Errorf("SeedHealth reaches %q, want Verifying", tr.Next)
			}
		}
		if tr.State == "AwaitingRequest" && tr.Signal == "SeedHealth" && tr.Action == "apply_instance" {
			t.Error("the rollout read drives apply_instance; a poll applies nothing")
		}
	}
	if !seeded {
		t.Fatal("no AwaitingRequest + SeedHealth transition; the read-only leg does not exist")
	}
}

// The rollout counts hop tests (GH-686). The executor reads ready, desired, and
// revision off the Deployment (srd006 R3.3); each hop between it and the panel
// must carry them or the panel renders "undefined/undefined ready (rev
// undefined)" from an interface that promises four fields and delivers one.

// clientOperation is one declared REST client operation's response mapping.
type clientOperation struct {
	Method  string `yaml:"method"`
	Path    string `yaml:"path"`
	Success struct {
		Status []int  `yaml:"status"`
		Signal string `yaml:"signal"`
	} `yaml:"success"`
	Response struct {
		Output map[string]string `yaml:"output"`
	} `yaml:"response"`
}

type clientRest struct {
	Rest struct {
		Clients map[string]struct {
			Operations map[string]clientOperation `yaml:"operations"`
		} `yaml:"clients"`
	} `yaml:"rest"`
}

// wordOutputProperties returns one declared word's output schema properties.
func wordOutputProperties(t *testing.T, agent, file, word string) map[string]map[string]string {
	t.Helper()
	var decls struct {
		Tools []struct {
			Name   string `yaml:"name"`
			Output struct {
				Schema struct {
					Properties map[string]map[string]string `yaml:"properties"`
				} `yaml:"schema"`
			} `yaml:"output"`
		} `yaml:"tools"`
	}
	readIntakeYAML(t, filepath.Join(agentDir(t, agent), file), &decls)
	for _, tool := range decls.Tools {
		if tool.Name == word {
			return tool.Output.Schema.Properties
		}
	}
	t.Fatalf("%s declares no %s word", agent, word)
	return nil
}

func clientOperationNamed(t *testing.T, agent, client, operation string) clientOperation {
	t.Helper()
	var rest clientRest
	readIntakeYAML(t, filepath.Join(agentDir(t, agent), "rest.yaml"), &rest)
	declared, ok := rest.Rest.Clients[client]
	if !ok {
		t.Fatalf("%s declares no %s client", agent, client)
	}
	op, ok := declared.Operations[operation]
	if !ok {
		t.Fatalf("%s's %s client declares no %s operation", agent, client, operation)
	}
	return op
}

// rolloutCountFields are the three fields the executor added beyond the phase.
var rolloutCountFields = []string{"ready", "desired", "revision"}

// TestRolloutCountsSurviveTheCreatorHop proves the creator maps the counts off
// the executor's response and re-serves them, rather than discarding everything
// but the phase.
func TestRolloutCountsSurviveTheCreatorHop(t *testing.T) {
	op := clientOperationNamed(t, "creator", "deployment_api", "read_rollout")
	for _, field := range rolloutCountFields {
		if got := op.Response.Output[field]; got != "$."+field {
			t.Errorf("read_rollout maps %s = %q, want $.%s; the executor serves it and the creator drops it", field, got, field)
		}
	}
	if op.Response.Output["health"] != "$.phase" {
		t.Errorf("read_rollout maps health = %q, want $.phase", op.Response.Output["health"])
	}

	properties := wordOutputProperties(t, "creator", "request-declarations.yaml", "verify_health")
	for _, field := range rolloutCountFields {
		if properties[field]["type"] != "integer" {
			t.Errorf("verify_health output schema declares %s as %q, want integer", field, properties[field]["type"])
		}
	}

	var rest rolloutRest
	readIntakeYAML(t, filepath.Join(agentDir(t, "creator"), "rest.yaml"), &rest)
	reported, ok := rest.Rest.Servers["creator_instance"].Endpoints["rollout"].
		MachineRequest.Response.TerminalStates["HealthReported"]
	if !ok {
		t.Fatal("creator rollout does not map HealthReported")
	}
	for _, field := range rolloutCountFields {
		if got := reported.Body[field]; got != "$.mapped."+field {
			t.Errorf("creator rollout body %s = %q, want $.mapped.%s", field, got, field)
		}
	}
}

// TestRolloutCountsSurviveTheCoordinatorHop proves the same for the hop the
// panel actually polls.
func TestRolloutCountsSurviveTheCoordinatorHop(t *testing.T) {
	op := clientOperationNamed(t, "coordinator", "creator", "creator_rollout_status")
	for _, field := range rolloutCountFields {
		if got := op.Response.Output[field]; got != "$."+field {
			t.Errorf("creator_rollout_status maps %s = %q, want $.%s", field, got, field)
		}
	}

	properties := wordOutputProperties(t, "coordinator", "request-declarations.yaml", "read_creator_rollout")
	for _, field := range rolloutCountFields {
		if properties[field]["type"] != "integer" {
			t.Errorf("read_creator_rollout output schema declares %s as %q, want integer", field, properties[field]["type"])
		}
	}

	var rest rolloutRest
	readIntakeYAML(t, filepath.Join(agentDir(t, "coordinator"), "rest.yaml"), &rest)
	read, ok := rest.Rest.Servers["coordinator_intent"].Endpoints["rollout"].
		MachineRequest.Response.TerminalStates["StatusRead"]
	if !ok {
		t.Fatal("coordinator rollout does not map StatusRead")
	}
	for _, field := range rolloutCountFields {
		if got := read.Body[field]; got != "$.mapped."+field {
			t.Errorf("coordinator rollout body %s = %q, want $.mapped.%s", field, got, field)
		}
	}
}

// TestPanelRolloutInterfaceMatchesWhatIsServed proves the panel's RolloutStatus
// declares exactly the fields the coordinator's poll serves. A type promising a
// field no hop carries reads as a measurement in the UI and renders undefined;
// the fix is to serve it or to stop declaring it, not to fabricate a zero.
func TestPanelRolloutInterfaceMatchesWhatIsServed(t *testing.T) {
	// agentDir walks up to the mesh root, so the panel sits two levels above it.
	meshRoot := filepath.Dir(filepath.Dir(agentDir(t, "coordinator")))
	source, err := os.ReadFile(filepath.Join(meshRoot, "ux", "app", "src", "provisioningApi.ts"))
	if err != nil {
		t.Fatalf("read provisioningApi.ts: %v", err)
	}
	block := regexp.MustCompile(`(?s)export interface RolloutStatus \{(.*?)\}`).FindSubmatch(source)
	if block == nil {
		t.Fatal("provisioningApi.ts declares no RolloutStatus")
	}
	declared := map[string]bool{}
	for _, field := range regexp.MustCompile(`(?m)^\s*(\w+)(\??):`).FindAllSubmatch(block[1], -1) {
		declared[string(field[1])] = string(field[2]) == "?"
	}

	var rest rolloutRest
	readIntakeYAML(t, filepath.Join(agentDir(t, "coordinator"), "rest.yaml"), &rest)
	served := rest.Rest.Servers["coordinator_intent"].Endpoints["rollout"].
		MachineRequest.Response.TerminalStates["StatusRead"].Body

	for field, optional := range declared {
		if _, ok := served[field]; !ok && !optional {
			t.Errorf("RolloutStatus requires %s, which the rollout poll never serves; serve it or narrow the interface", field)
		}
	}
	for field := range served {
		// schema_version is contract metadata the panel does not render.
		if field == "schema_version" {
			continue
		}
		if _, ok := declared[field]; !ok {
			t.Errorf("the rollout poll serves %s, which RolloutStatus does not declare", field)
		}
	}
}
