// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

type documentationRequestMachine struct {
	Transitions []struct {
		State  string `yaml:"state"`
		Signal string `yaml:"signal"`
		Next   string `yaml:"next"`
		Action string `yaml:"action"`
	} `yaml:"transitions"`
}

type documentationRESTConfig struct {
	Rest struct {
		Servers map[string]struct {
			Endpoints map[string]struct {
				Method         string `yaml:"method"`
				Path           string `yaml:"path"`
				Binding        string `yaml:"binding"`
				MachineRequest struct {
					InitialSignal string `yaml:"initial_signal"`
					Request       struct {
						Body map[string]string `yaml:"body"`
						Path map[string]string `yaml:"path"`
					} `yaml:"request"`
					Response struct {
						TerminalStates map[string]interface{} `yaml:"terminal_states"`
					} `yaml:"response"`
				} `yaml:"machine_request"`
			} `yaml:"endpoints"`
		} `yaml:"servers"`
	} `yaml:"rest"`
}

func TestDocumentationRequestMachineSequencesConfiguredActions(t *testing.T) {
	var machine documentationRequestMachine
	readDocumentationCuratorYAML(t, "request-machine.yaml", &machine)

	for signal, action := range map[string]string{
		"ValidateRequested": "doc_validate",
		"SuggestRequested":  "doc_suggest_changes",
		"ApproveRequested":  "doc_patch_approve",
		"RejectRequested":   "doc_patch_reject",
	} {
		requireDocumentationTransition(t, machine, signal, action)
	}
}

func TestDocumentationActionEndpointsUseMachineRequestBinding(t *testing.T) {
	var config documentationRESTConfig
	readDocumentationCuratorYAML(t, "rest.yaml", &config)
	endpoints := config.Rest.Servers["documentation_curator_requests"].Endpoints

	for name, want := range map[string]struct {
		method string
		path   string
		signal string
	}{
		"validate_action": {"POST", "/api/v1/actions/validate", "ValidateRequested"},
		"suggest_action":  {"POST", "/api/v1/actions/suggest", "SuggestRequested"},
		"approve_action":  {"POST", "/api/v1/actions/patches/{patch_id}/approve", "ApproveRequested"},
		"reject_action":   {"POST", "/api/v1/actions/patches/{patch_id}/reject", "RejectRequested"},
	} {
		endpoint, ok := endpoints[name]
		if !ok {
			t.Fatalf("missing endpoint %q", name)
		}
		if endpoint.Method != want.method || endpoint.Path != want.path ||
			endpoint.Binding != "machine_request" || endpoint.MachineRequest.InitialSignal != want.signal {
			t.Fatalf("endpoint %q = %+v", name, endpoint)
		}
		for _, terminal := range []string{"ActionCompleted", "ActionRejected", "Failed"} {
			if _, ok := endpoint.MachineRequest.Response.TerminalStates[terminal]; !ok {
				t.Errorf("endpoint %q missing terminal state response %q", name, terminal)
			}
		}
	}
}

func readDocumentationCuratorYAML(t *testing.T, name string, target any) {
	t.Helper()
	path := filepath.Join("..", "agents", "knowledge-manager", "documentation-curator", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(data, target); err != nil {
		t.Fatal(err)
	}
}

func requireDocumentationTransition(t *testing.T, machine documentationRequestMachine, signal, action string) {
	t.Helper()
	for _, transition := range machine.Transitions {
		if transition.State == "AwaitingRequest" && transition.Signal == signal {
			if transition.Action != action {
				t.Fatalf("%s action = %q, want %q", signal, transition.Action, action)
			}
			return
		}
	}
	t.Fatalf("missing AwaitingRequest/%s transition", signal)
}
