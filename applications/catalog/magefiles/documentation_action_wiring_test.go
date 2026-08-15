// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

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
		Clients map[string]struct {
			Operations map[string]struct {
				Failures []struct {
					Status []int  `yaml:"status"`
					Signal string `yaml:"signal"`
				} `yaml:"failures"`
			} `yaml:"operations"`
		} `yaml:"clients"`
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

type documentationUXConfig struct {
	Actions map[string]struct {
		UIAction             string `yaml:"ui_action"`
		RequestMachineAction string `yaml:"request_machine_action"`
		RequestMachineRoute  string `yaml:"request_machine_route"`
	} `yaml:"actions"`
}

func TestDocumentationRequestMachineSequencesConfiguredActions(t *testing.T) {
	var machine documentationRequestMachine
	readDocumentationCuratorYAML(t, "request-machine.yaml", &machine)

	for signal, action := range map[string]string{
		"ValidateRequested": "capture_validation",
		"SuggestRequested":  "capture_suggestion",
		"ApproveRequested":  "capture_approval",
		"RejectRequested":   "capture_rejection",
	} {
		requireDocumentationTransition(t, machine, signal, action)
	}
	for _, action := range []string{"validate_specs", "build_suggestion", "review_pending", "build_approval", "write_review"} {
		requireDocumentationAction(t, machine, action)
	}
}

func TestDocumentationActionEndpointsUseMachineRequestBinding(t *testing.T) {
	var config documentationRESTConfig
	readDocumentationCuratorYAML(t, "rest.yaml", &config)
	endpoints := config.Rest.Servers["documentation_curator"].Endpoints

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
		terminals := map[string][]string{
			"validate_action": {"ValidationReady", "ValidationRejected", "Failed"},
			"suggest_action":  {"SuggestionReady", "Failed"},
			"approve_action":  {"DecisionReady", "ActionRejected", "Failed"},
			"reject_action":   {"DecisionReady", "ActionRejected", "Failed"},
		}[name]
		for _, terminal := range terminals {
			if _, ok := endpoint.MachineRequest.Response.TerminalStates[terminal]; !ok {
				t.Errorf("endpoint %q missing terminal state response %q", name, terminal)
			}
		}
	}
}

func requireDocumentationAction(t *testing.T, machine documentationRequestMachine, action string) {
	t.Helper()
	for _, transition := range machine.Transitions {
		if transition.Action == action {
			return
		}
	}
	t.Fatalf("missing declarative action %s", action)
}

func TestDocumentationPatchDecisionClientsMapMissingPatches(t *testing.T) {
	var config documentationRESTConfig
	readDocumentationCuratorYAML(t, "rest.yaml", &config)
	operations := config.Rest.Clients["documentation"].Operations

	for _, name := range []string{"doc_patch_approve", "doc_patch_reject"} {
		operation, ok := operations[name]
		if !ok {
			t.Fatalf("missing operation %q", name)
		}
		found := false
		for _, failure := range operation.Failures {
			if len(failure.Status) == 1 && failure.Status[0] == 404 && failure.Signal == "RESTMissing" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("operation %q does not map status 404 to RESTMissing", name)
		}
	}
}

func TestDocumentationUXActionsUseRequestMachineRoutes(t *testing.T) {
	var ux documentationUXConfig
	readDocumentationCuratorYAML(t, filepath.Join("ui", "ux.yaml"), &ux)

	for name, want := range map[string]struct {
		ui, machine, route string
	}{
		"validate_document": {"doc_validate", "validate_specs", "/actions/validate"},
		"suggest_changes":   {"doc_suggest_changes", "build_suggestion", "/actions/suggest"},
		"approve_patch":     {"doc_patch_approve", "build_approval", "/actions/patches/{patch_id}/approve"},
		"reject_patch":      {"doc_patch_reject", "build_approval", "/actions/patches/{patch_id}/reject"},
	} {
		action, ok := ux.Actions[name]
		if !ok {
			t.Fatalf("missing UX action %q", name)
		}
		if action.UIAction != want.ui || action.RequestMachineAction != want.machine ||
			action.RequestMachineRoute != want.route {
			t.Fatalf("UX action %q = %+v", name, action)
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
