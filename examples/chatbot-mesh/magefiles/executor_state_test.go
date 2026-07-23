// Copyright (c) 2026 Nokia. All rights reserved.

package main

import "testing"

// These cover the executor's state read (srd006 R1.5, GH-752). The provisioning
// panel's initial load must work from a fresh install with no prior apply, so
// this reads the release's fully computed values (chart defaults merged with any
// applied overrides) via `helm get values --all -o json` rather than the
// executor's own overrides file, which exists only after the first apply.

func executorStateEndpoint(t *testing.T) rolloutEndpoint {
	t.Helper()
	var rest rolloutRest
	readIntakeYAML(t, agentDir(t, "executor")+"/rest.yaml", &rest)
	server, ok := rest.Rest.Servers["executor_apply"]
	if !ok {
		t.Fatal("executor_apply server not declared")
	}
	endpoint, ok := server.Endpoints["state"]
	if !ok {
		t.Fatal("executor declares no state endpoint; the provisioning panel has nothing to load")
	}
	return endpoint
}

func executorStateMachine(t *testing.T) rolloutMachine {
	t.Helper()
	endpoint := executorStateEndpoint(t)
	if endpoint.MachineRequest.Machine == "" {
		t.Fatal("state endpoint names no machine")
	}
	var machine rolloutMachine
	readIntakeYAML(t, agentDir(t, "executor")+"/"+endpoint.MachineRequest.Machine, &machine)
	return machine
}

// TestExecutorStateMachineReachesTwoTerminals proves the machine reaches Read and
// Unavailable, and that helm_get_values is the word the Seed dispatches -- an
// unreadable release must land in Unavailable rather than falling through.
func TestExecutorStateMachineReachesTwoTerminals(t *testing.T) {
	machine := executorStateMachine(t)

	for _, want := range []string{"Read", "Unavailable"} {
		if !containsString(machine.TerminalStates, want) {
			t.Errorf("state machine does not declare %s terminal", want)
		}
	}

	var seedAction string
	unavailableFrom := map[string]bool{}
	readFrom := map[string]bool{}
	for _, tr := range machine.Transitions {
		if tr.State == machine.InitialState && tr.Signal == "Seed" {
			seedAction = tr.Action
		}
		if tr.Next == "Unavailable" && tr.Signal == "ToolFailed" {
			unavailableFrom[tr.State] = true
		}
		if tr.Next == "Read" && tr.Signal == "ToolDone" {
			readFrom[tr.State] = true
		}
	}
	if seedAction != "helm_get_values" {
		t.Errorf("seed action = %q, want helm_get_values", seedAction)
	}
	if !unavailableFrom["Reading"] {
		t.Error("Reading + ToolFailed does not reach Unavailable; a failed read would fall through")
	}
	if !readFrom["Reading"] {
		t.Error("Reading + ToolDone does not reach Read")
	}
}

// TestExecutorStateEndpointMapsTwoOutcomes proves the two terminal states map to
// two responses: a broken read answers 502 with no mesh-view fields, so the panel
// never renders a topology-less mesh as if it were real.
func TestExecutorStateEndpointMapsTwoOutcomes(t *testing.T) {
	endpoint := executorStateEndpoint(t)
	states := endpoint.MachineRequest.Response.TerminalStates
	if len(states) == 0 {
		t.Fatal("state maps no terminal states; helm_get_values emits only ToolDone and ToolFailed, so a signal cannot separate the two outcomes")
	}

	read, ok := states["Read"]
	if !ok {
		t.Fatal("state does not map Read")
	}
	if read.Status != 200 {
		t.Errorf("Read status = %d, want 200", read.Status)
	}
	for _, field := range []string{
		"rags", "llmInCluster", "llmExternalURL", "llmChatModel", "llmEmbedModel",
		"llmChatModels", "llmRouterModel", "llmTopology",
		"paramsNResults", "paramsChunkCap", "paramsRouterDefault",
	} {
		if _, present := read.Body[field]; !present {
			t.Errorf("Read body does not map %s; the panel's MeshView needs it", field)
		}
	}

	unavailable, ok := states["Unavailable"]
	if !ok {
		t.Fatal("state does not map Unavailable")
	}
	if unavailable.Status != 502 {
		t.Errorf("Unavailable status = %d, want 502; a broken read must not answer 200 with a mapped-empty view", unavailable.Status)
	}
	for _, field := range []string{"rags", "llmInCluster", "paramsNResults"} {
		if _, present := unavailable.Body[field]; present {
			t.Errorf("Unavailable carries %s; the whole point is that no mesh view is reportable", field)
		}
	}
}

// TestExecutorStateTerminalMappingsAgree proves the mapped terminal states and the
// machine's declared terminal states are the same set (agent-core srd030 R4.8).
func TestExecutorStateTerminalMappingsAgree(t *testing.T) {
	machine := executorStateMachine(t)
	mapped := executorStateEndpoint(t).MachineRequest.Response.TerminalStates
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

// TestExecutorGetValuesWordContract proves helm_get_values reads the installed
// release's fully computed values as JSON, rather than a baked default or the
// user-supplied overrides alone (which would omit chart defaults on a fresh
// install with no prior apply).
func TestExecutorGetValuesWordContract(t *testing.T) {
	word := executorExecWord(t, "helm_get_values")

	if word.Binary != "helm" {
		t.Errorf("binary = %q, want helm", word.Binary)
	}
	if !containsString(word.Args, "get") || !containsString(word.Args, "values") {
		t.Errorf("args %v do not read values", word.Args)
	}
	if !containsString(word.Args, "--all") {
		t.Errorf("args %v omit --all; without it, chart defaults never applied by an operator would be missing on a fresh install", word.Args)
	}
	if !containsString(word.Args, "-o") || !containsString(word.Args, "json") {
		t.Errorf("args %v do not request JSON output; the response body selectors need parseable fields, not CLI prose", word.Args)
	}
	if !containsString(word.Args, "--namespace") || !containsString(word.Args, "default") {
		t.Errorf("args %v do not name the namespace placeholder the chart rewrites", word.Args)
	}

	for _, signal := range []string{"ToolDone", "ToolFailed"} {
		if !containsString(word.Emits, signal) {
			t.Errorf("emits %v missing %s", word.Emits, signal)
		}
	}
	if len(word.Errors) == 0 {
		t.Error("helm_get_values declares no errors")
	}
	for _, e := range word.Errors {
		if e.Signal != "ToolFailed" || e.Condition == "" {
			t.Errorf("declared error %+v does not name a ToolFailed condition", e)
		}
	}
}
