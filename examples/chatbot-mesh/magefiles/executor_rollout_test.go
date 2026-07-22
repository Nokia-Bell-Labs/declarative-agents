// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"text/template"
)

// These cover the executor's rollout read (GH-686). `kubectl rollout status`
// exits non-zero both for a rollout that is still progressing and for one it
// could not read at all -- an absent Deployment, a wrong kubeconfig, a denied
// RBAC read, an unreachable API server -- so a phase taken from that exit code
// alone reported a wholly broken read as ongoing progress, in a panel that polls
// every 3s. A second read of the Deployment object separates the two and carries
// the counts the panel's RolloutStatus declares (srd006 R3.3).

type rolloutResponseMapping struct {
	Status int               `yaml:"status"`
	Body   map[string]string `yaml:"body"`
}

type rolloutEndpoint struct {
	Method         string `yaml:"method"`
	Path           string `yaml:"path"`
	MachineRequest struct {
		Machine  string `yaml:"machine"`
		Response struct {
			TerminalStates  map[string]rolloutResponseMapping `yaml:"terminal_states"`
			TerminalSignals map[string]rolloutResponseMapping `yaml:"terminal_signals"`
		} `yaml:"response"`
	} `yaml:"machine_request"`
}

type rolloutRest struct {
	Rest struct {
		Servers map[string]struct {
			Endpoints map[string]rolloutEndpoint `yaml:"endpoints"`
		} `yaml:"servers"`
	} `yaml:"rest"`
}

type rolloutMachine struct {
	InitialState string `yaml:"initial_state"`
	Budget       struct {
		MaxIterations int `yaml:"max_iterations"`
	} `yaml:"budget"`
	States []struct {
		Name string `yaml:"name"`
	} `yaml:"states"`
	TerminalStates []string `yaml:"terminal_states"`
	Transitions    []struct {
		State  string `yaml:"state"`
		Signal string `yaml:"signal"`
		Next   string `yaml:"next"`
		Action string `yaml:"action"`
	} `yaml:"transitions"`
}

type execDeclaration struct {
	Name   string   `yaml:"name"`
	Binary string   `yaml:"binary"`
	Args   []string `yaml:"args"`
	Emits  []string `yaml:"emits"`
	Output struct {
		Schema struct {
			Properties map[string]map[string]string `yaml:"properties"`
			Required   []string                     `yaml:"required"`
		} `yaml:"schema"`
	} `yaml:"output"`
	Errors []struct {
		Signal    string `yaml:"signal"`
		Condition string `yaml:"condition"`
	} `yaml:"errors"`
	Relationships map[string]any `yaml:"relationships"`
}

type execDeclarations struct {
	Tools []execDeclaration `yaml:"tools"`
}

func executorRolloutEndpoint(t *testing.T) rolloutEndpoint {
	t.Helper()
	var rest rolloutRest
	readIntakeYAML(t, filepath.Join(agentDir(t, "executor"), "rest.yaml"), &rest)
	server, ok := rest.Rest.Servers["executor_apply"]
	if !ok {
		t.Fatal("executor_apply server not declared")
	}
	endpoint, ok := server.Endpoints["rollout"]
	if !ok {
		t.Fatal("executor declares no rollout endpoint; the creator has nothing to read")
	}
	return endpoint
}

func executorRolloutMachine(t *testing.T) rolloutMachine {
	t.Helper()
	endpoint := executorRolloutEndpoint(t)
	if endpoint.MachineRequest.Machine == "" {
		t.Fatal("rollout endpoint names no machine")
	}
	var machine rolloutMachine
	readIntakeYAML(t, filepath.Join(agentDir(t, "executor"), endpoint.MachineRequest.Machine), &machine)
	return machine
}

func executorExecWord(t *testing.T, name string) execDeclaration {
	t.Helper()
	var decls execDeclarations
	readIntakeYAML(t, filepath.Join(agentDir(t, "executor"), "exec-declarations.yaml"), &decls)
	for _, tool := range decls.Tools {
		if tool.Name == name {
			return tool
		}
	}
	t.Fatalf("executor declares no %s exec word", name)
	return execDeclaration{}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// TestExecutorRolloutMachineSeparatesBrokenRead proves the machine reaches three
// terminal outcomes, not two: a failed counts read lands in Unavailable from
// either phase leg rather than joining the progressing one.
func TestExecutorRolloutMachineSeparatesBrokenRead(t *testing.T) {
	machine := executorRolloutMachine(t)

	for _, want := range []string{"Complete", "Progressing", "Unavailable"} {
		if !containsString(machine.TerminalStates, want) {
			t.Errorf("rollout machine does not declare %s terminal; three outcomes cannot map to three responses", want)
		}
	}

	// A counts read that fails must land in Unavailable from both phase legs.
	unavailableFrom := map[string]bool{}
	countsDispatchedFrom := map[string]bool{}
	var pollAction string
	for _, tr := range machine.Transitions {
		if tr.Next == "Unavailable" && tr.Signal == "ToolFailed" {
			unavailableFrom[tr.State] = true
		}
		if tr.Action == "kubectl_get_rollout_counts" {
			countsDispatchedFrom[tr.State] = true
		}
		if tr.State == machine.InitialState && tr.Signal == "Seed" {
			pollAction = tr.Action
		}
	}
	for _, state := range []string{"ReadingComplete", "ReadingProgressing"} {
		if !unavailableFrom[state] {
			t.Errorf("%s + ToolFailed does not reach Unavailable; a broken read would report a phase", state)
		}
	}
	if !countsDispatchedFrom["Polling"] {
		t.Error("the counts read is not dispatched from Polling; both phase legs must read the Deployment")
	}
	if len(countsDispatchedFrom) != 1 {
		t.Errorf("the counts read is dispatched from %v; it belongs on the Polling legs alone", countsDispatchedFrom)
	}

	// Order matters: the poll runs first because the terminal state carries the
	// phase, and the response body maps the counts from the last word's output.
	if pollAction != "kubectl_rollout_poll" {
		t.Errorf("initial action = %q, want kubectl_rollout_poll (the phase read runs first)", pollAction)
	}
	if machine.Budget.MaxIterations < 5 {
		t.Errorf("max_iterations = %d, too small for two dispatches plus the terminal step",
			machine.Budget.MaxIterations)
	}
}

// TestExecutorRolloutEndpointMapsThreeOutcomes proves the three terminal states
// map to three responses: a broken read answers 502, and both live phases carry
// the counts the panel's RolloutStatus declares.
func TestExecutorRolloutEndpointMapsThreeOutcomes(t *testing.T) {
	endpoint := executorRolloutEndpoint(t)
	states := endpoint.MachineRequest.Response.TerminalStates
	if len(states) == 0 {
		t.Fatal("rollout maps no terminal states; both exec words emit only ToolDone and ToolFailed, so signals cannot separate three outcomes")
	}
	if len(endpoint.MachineRequest.Response.TerminalSignals) > 0 {
		t.Error("rollout still maps terminal signals; the counts word emits ToolDone for a complete and a progressing rollout alike")
	}

	for phase, state := range map[string]string{"complete": "Complete", "progressing": "Progressing"} {
		mapping, ok := states[state]
		if !ok {
			t.Errorf("rollout does not map terminal state %s", state)
			continue
		}
		if mapping.Status != 200 {
			t.Errorf("%s status = %d, want 200", state, mapping.Status)
		}
		if mapping.Body["phase"] != phase {
			t.Errorf("%s phase = %q, want %q", state, mapping.Body["phase"], phase)
		}
		// The counts come from the last word's output, which is the Deployment
		// read, so each field maps a selector rather than a literal.
		for _, field := range []string{"ready", "desired", "revision"} {
			if got := mapping.Body[field]; got != "$."+field {
				t.Errorf("%s body %s = %q, want $.%s; the panel renders the counts it declares", state, field, got, field)
			}
		}
	}

	unavailable, ok := states["Unavailable"]
	if !ok {
		t.Fatal("rollout does not map Unavailable; a read the cluster cannot serve would fall through")
	}
	if unavailable.Status != 502 {
		t.Errorf("Unavailable status = %d, want 502; a broken read must not answer 200 with a phase", unavailable.Status)
	}
	if _, present := unavailable.Body["phase"]; present {
		t.Error("Unavailable carries a phase; the whole point is that no phase is reportable")
	}
}

// TestExecutorRolloutTerminalMappingsAgree proves the mapped terminal states and
// the machine's declared terminal states are the same set: an unmapped terminal
// falls through at runtime and a mapped non-terminal fails config validation
// (agent-core srd030 R4.8).
func TestExecutorRolloutTerminalMappingsAgree(t *testing.T) {
	machine := executorRolloutMachine(t)
	mapped := executorRolloutEndpoint(t).MachineRequest.Response.TerminalStates
	for _, state := range machine.TerminalStates {
		if _, ok := mapped[state]; !ok {
			t.Errorf("terminal state %s is unmapped; the request would answer response_missing", state)
		}
	}
	for state := range mapped {
		if !containsString(machine.TerminalStates, state) {
			t.Errorf("mapped state %s is not declared terminal; config validation rejects it", state)
		}
	}
}

// TestExecutorRolloutCountsWordContract proves the counts word targets the
// installed release rather than a baked default (GH-484) and carries the full
// contract metadata, errors and relationships included (GH-691).
func TestExecutorRolloutCountsWordContract(t *testing.T) {
	word := executorExecWord(t, "kubectl_get_rollout_counts")

	if word.Binary != "kubectl" {
		t.Errorf("binary = %q, want kubectl", word.Binary)
	}
	// The placeholder coordinates the chart rewrites per release; the executor
	// render test proves the rewrite reaches them.
	if !containsString(word.Args, "deployment/chatbot-mesh-chatbot") {
		t.Errorf("args %v do not name the chatbot Deployment placeholder the chart rewrites", word.Args)
	}
	if !containsString(word.Args, "--namespace") || !containsString(word.Args, "default") {
		t.Errorf("args %v do not name the namespace placeholder the chart rewrites", word.Args)
	}

	for _, signal := range []string{"ToolDone", "ToolFailed"} {
		if !containsString(word.Emits, signal) {
			t.Errorf("emits %v missing %s", word.Emits, signal)
		}
	}
	for _, field := range []string{"ready", "desired", "revision"} {
		if _, ok := word.Output.Schema.Properties[field]; !ok {
			t.Errorf("output schema declares no %s; the response body maps $.%s from this word", field, field)
		}
		if !containsString(word.Output.Schema.Required, field) {
			t.Errorf("output schema does not require %s", field)
		}
	}

	if len(word.Errors) == 0 {
		t.Error("the counts word declares no errors; a failure signal with no declared condition is the GH-691 gap")
	}
	for _, e := range word.Errors {
		if e.Signal != "ToolFailed" || e.Condition == "" {
			t.Errorf("declared error %+v does not name a ToolFailed condition", e)
		}
	}
	if len(word.Relationships) == 0 {
		t.Error("the counts word declares no relationships; its order against kubectl_rollout_poll is what makes the counts the last output")
	}
}

// TestExecutorRolloutCountsRenderJSON runs the declared go-template over the
// Deployment shapes a rollout passes through and proves each renders parseable
// JSON with the declared fields. A Deployment carries no readyReplicas until a
// replica is ready and no revision annotation until the controller writes one,
// so an ungated field would emit `<no value>` mid-object and the response body
// selectors would find nothing. The template runs with missingkey=zero, which is
// what kubectl's --allow-missing-template-keys=true sets.
func TestExecutorRolloutCountsRenderJSON(t *testing.T) {
	word := executorExecWord(t, "kubectl_get_rollout_counts")
	var raw string
	for _, arg := range word.Args {
		if strings.HasPrefix(arg, "go-template=") {
			raw = strings.TrimPrefix(arg, "go-template=")
		}
	}
	if raw == "" {
		t.Fatal("the counts word declares no go-template; the counts would come back as opaque CLI prose")
	}
	tmpl, err := template.New("counts").Option("missingkey=zero").Parse(raw)
	if err != nil {
		t.Fatalf("parse declared go-template: %v", err)
	}

	cases := []struct {
		name                     string
		object                   string
		ready, desired, revision float64
	}{
		{
			name:   "rolled out",
			object: `{"metadata":{"annotations":{"deployment.kubernetes.io/revision":"3"}},"spec":{"replicas":2},"status":{"readyReplicas":2}}`,
			ready:  2, desired: 2, revision: 3,
		},
		{
			name:   "no replica ready yet",
			object: `{"metadata":{"annotations":{"deployment.kubernetes.io/revision":"1"}},"spec":{"replicas":2},"status":{}}`,
			ready:  0, desired: 2, revision: 1,
		},
		{
			name:   "freshly created, no revision annotation",
			object: `{"metadata":{},"spec":{"replicas":1},"status":{}}`,
			ready:  0, desired: 1, revision: 0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var object map[string]any
			if err := json.Unmarshal([]byte(tc.object), &object); err != nil {
				t.Fatalf("sample Deployment: %v", err)
			}
			var rendered strings.Builder
			if err := tmpl.Execute(&rendered, object); err != nil {
				t.Fatalf("render: %v", err)
			}
			var counts map[string]any
			if err := json.Unmarshal([]byte(rendered.String()), &counts); err != nil {
				t.Fatalf("rendered %q is not JSON: %v", rendered.String(), err)
			}
			for field, want := range map[string]float64{
				"ready": tc.ready, "desired": tc.desired, "revision": tc.revision,
			} {
				if got, ok := counts[field].(float64); !ok || got != want {
					t.Errorf("%s = %v, want %v (rendered %q)", field, counts[field], want, rendered.String())
				}
			}
		})
	}
}
