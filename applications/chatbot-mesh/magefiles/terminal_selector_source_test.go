// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// terminalSourceRest reads the two halves this check compares: the client
// operations, which declare what a word's output carries, and the endpoints,
// whose terminal bodies select from it.
type terminalSourceRest struct {
	Rest struct {
		Clients map[string]struct {
			Operations map[string]struct {
				Params struct {
					CarryForward []string `yaml:"carry_forward"`
				} `yaml:"params"`
				Success struct {
					Signal string `yaml:"signal"`
				} `yaml:"success"`
				Failures []struct {
					Signal string `yaml:"signal"`
				} `yaml:"failures"`
				Response struct {
					Output map[string]string `yaml:"output"`
				} `yaml:"response"`
			} `yaml:"operations"`
		} `yaml:"clients"`
		Servers map[string]struct {
			Endpoints map[string]struct {
				MachineRequest struct {
					Response struct {
						TerminalSignals map[string]terminalResponse `yaml:"terminal_signals"`
					} `yaml:"response"`
				} `yaml:"machine_request"`
			} `yaml:"endpoints"`
		} `yaml:"servers"`
	} `yaml:"rest"`
}

// clientOperationOutput is what one client operation's result offers a terminal
// body: the keys of its response mapping and its carry_forward list.
type clientOperationOutput struct {
	name    string
	mapped  map[string]bool
	carried map[string]bool
}

// TestTerminalBodySelectorsNameDeclaredOutput proves the sub-map selectors in
// terminal response bodies are sourced. TestTerminalResponseBodiesUseResolvableSelectors
// checks the selector's form -- that it is a `$.` and not a `$from(...)` that
// would ship verbatim -- but a well-formed selector still resolves to nothing
// when it names a field the terminal word does not carry, and the field then
// answers null where the contract promised a value.
//
// Only `$.mapped.*` and `$.carried.*` are checked, because those two are
// declared in the operation: mapped comes from its response.output and carried
// from its carry_forward. Top-level selectors ($.status, $.operation,
// $.failure_stage) come from the result envelope agent-core builds for every
// REST client word and are not declared per operation, so there is nothing here
// to compare them against.
//
// The endpoint must key by terminal signal for this to be decidable: a terminal
// state may be reached by several words, while a signal names the operation
// that emitted it (GH-218).
//
// One emitter declaring the field is enough. A signal can be emitted by the
// operations of more than one leg -- Reconfigured comes from both the rollout
// that follows an ingest and the values-only rollout that does not -- and the
// provision body reads the ingest count on the leg that has one, which is
// absent rather than wrong on the other. Requiring every emitter to declare it
// would call that intended asymmetry a defect.
func TestTerminalBodySelectorsNameDeclaredOutput(t *testing.T) {
	for _, agent := range meshAgentsWithRest(t) {
		var rest terminalSourceRest
		readIntakeYAML(t, filepath.Join(agentDir(t, agent), "rest.yaml"), &rest)

		// Index every client operation by the signals it emits. A signal
		// emitted by more than one operation is ambiguous, and the check
		// requires the field to be satisfied by all of them.
		bySignal := map[string][]clientOperationOutput{}
		for _, client := range rest.Rest.Clients {
			for name, operation := range client.Operations {
				out := clientOperationOutput{
					name:    name,
					mapped:  map[string]bool{},
					carried: map[string]bool{},
				}
				for field := range operation.Response.Output {
					out.mapped[field] = true
				}
				for _, field := range operation.Params.CarryForward {
					out.carried[field] = true
				}
				signals := []string{operation.Success.Signal}
				for _, failure := range operation.Failures {
					signals = append(signals, failure.Signal)
				}
				for _, signal := range signals {
					if signal == "" || signal == "CommandError" {
						continue
					}
					bySignal[signal] = append(bySignal[signal], out)
				}
			}
		}

		for serverName, server := range rest.Rest.Servers {
			for endpointName, endpoint := range server.Endpoints {
				for signal, response := range endpoint.MachineRequest.Response.TerminalSignals {
					emitters, known := bySignal[signal]
					if !known {
						continue
					}
					for field, value := range response.Body {
						selector, ok := value.(string)
						if !ok {
							continue
						}
						kind, name, ok := terminalSubSelector(selector)
						if !ok {
							continue
						}
						var sourced bool
						var candidates []string
						for _, emitter := range emitters {
							declared := emitter.mapped
							if kind == "carried" {
								declared = emitter.carried
							}
							candidates = append(candidates, emitter.name)
							if declared[name] {
								sourced = true
							}
						}
						if sourced {
							continue
						}
						t.Errorf(
							"%s %s/%s (signal %s): field %q selects %q, but no operation"+
								" emitting %s declares %s %q (%s) -- the selector resolves to"+
								" nothing and the field answers null where the contract"+
								" promises a value",
							agent, serverName, endpointName, signal, field, selector,
							signal, kind, name, strings.Join(candidates, ", "))
					}
				}
			}
		}
	}
}

// terminalSubSelector splits `$.mapped.count` or `$.carried.collection` into
// its half and field name. Anything else -- a literal, a top-level selector, a
// deeper path -- returns false and is left to the other checks.
func terminalSubSelector(selector string) (kind, name string, ok bool) {
	for _, prefix := range []string{"mapped", "carried"} {
		head := "$." + prefix + "."
		if !strings.HasPrefix(selector, head) {
			continue
		}
		name = strings.TrimPrefix(selector, head)
		if name == "" || strings.Contains(name, ".") {
			return "", "", false
		}
		return prefix, name, true
	}
	return "", "", false
}
