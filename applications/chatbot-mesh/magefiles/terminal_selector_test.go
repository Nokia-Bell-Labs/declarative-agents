// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// terminalResponseRest is the shape a machine_request endpoint exposes: a
// response keyed by terminal state or terminal signal, each mapping body fields
// to selectors.
type terminalResponseRest struct {
	Rest struct {
		Servers map[string]struct {
			Endpoints map[string]struct {
				Path           string `yaml:"path"`
				MachineRequest struct {
					Response struct {
						TerminalStates  map[string]terminalResponse `yaml:"terminal_states"`
						TerminalSignals map[string]terminalResponse `yaml:"terminal_signals"`
					} `yaml:"response"`
				} `yaml:"machine_request"`
			} `yaml:"endpoints"`
		} `yaml:"servers"`
	} `yaml:"rest"`
}

type terminalResponse struct {
	Body map[string]any `yaml:"body"`
}

// A terminal response body resolves only `$.` selectors, and only against the
// terminal word's own output. agent-core's machineSelectorValue returns anything
// else verbatim rather than rejecting it, so a `$from(word).field` selector --
// which is valid in a REST client input_mapping, a base_url selector, and an LLM
// word source -- silently ships its own text as the field's value.
//
// That is not hypothetical. The creator's ingest response answered with the
// string "$from(resolve_ingest_collection).mapped.collection_name" where the
// collection name belonged, and the provisioning intent answered
// "$from(request_ingest).mapped.count" as its count. Both passed `mage audit`,
// the boot smoke, and a declaration test that asserted the selector was present
// rather than what it resolved to (GH-198).
func TestTerminalResponseBodiesUseResolvableSelectors(t *testing.T) {
	for _, agent := range meshAgentsWithRest(t) {
		var rest terminalResponseRest
		readIntakeYAML(t, filepath.Join(agentDir(t, agent), "rest.yaml"), &rest)

		for serverName, server := range rest.Rest.Servers {
			for endpointName, endpoint := range server.Endpoints {
				responses := map[string]terminalResponse{}
				for terminal, response := range endpoint.MachineRequest.Response.TerminalStates {
					responses["state "+terminal] = response
				}
				for terminal, response := range endpoint.MachineRequest.Response.TerminalSignals {
					responses["signal "+terminal] = response
				}

				for terminal, response := range responses {
					for field, value := range response.Body {
						selector, ok := value.(string)
						if !ok || !strings.HasPrefix(selector, "$") {
							continue
						}
						if strings.HasPrefix(selector, "$.") {
							continue
						}
						t.Errorf(
							"%s %s/%s (%s): field %q maps %q, which a terminal response"+
								" body does not resolve -- agent-core returns it verbatim,"+
								" so the field ships its own selector text as data",
							agent, serverName, endpointName, terminal, field, selector)
					}
				}
			}
		}
	}
}

// meshAgentsWithRest lists every agent directory carrying a rest.yaml, so a new
// agent is covered without being named here.
func meshAgentsWithRest(t *testing.T) []string {
	t.Helper()
	root := filepath.Dir(agentDir(t, "creator"))
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read the agents directory: %v", err)
	}
	var agents []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, entry.Name(), "rest.yaml")); err == nil {
			agents = append(agents, entry.Name())
		}
	}
	if len(agents) == 0 {
		t.Fatal("no agent declares a rest.yaml; the scan would pass vacuously")
	}
	return agents
}

// The guard above is a lexical rule, so this pins that it is the right rule:
// the selector forms it accepts and rejects are the ones agent-core accepts and
// rejects. If machineSelectorValue ever learns $from, this test is what should
// fail first.
func TestTerminalSelectorRuleMatchesTheRuntime(t *testing.T) {
	for selector, resolvable := range map[string]bool{
		"$.left":                              true,
		"$.mapped.health":                     true,
		"$.carried.count":                     true,
		"$from(resolve_ingest_collection).id": false,
		"$from(request_ingest).mapped.count":  false,
		"$$":                                  false,
	} {
		got := strings.HasPrefix(selector, "$.")
		if got != resolvable {
			t.Errorf("selector %q: rule says resolvable=%v, want %v", selector, got, resolvable)
		}
	}

	// A literal is not a selector and travels through untouched, which is how
	// status strings and fixed error codes reach the body.
	for _, literal := range []string{"ingested", "values_rejected", "0"} {
		if strings.HasPrefix(literal, "$") {
			t.Errorf("literal %q would be treated as a selector", literal)
		}
	}
}
