// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"path/filepath"
	"testing"
	"time"
)

// endpointBudgets reads every machine_request request bound an agent declares,
// keyed by endpoint name.
type endpointBudgets struct {
	Rest struct {
		Servers map[string]struct {
			Endpoints map[string]struct {
				MachineRequest struct {
					Timeout string `yaml:"timeout"`
				} `yaml:"machine_request"`
			} `yaml:"endpoints"`
		} `yaml:"servers"`
	} `yaml:"rest"`
}

func endpointBudget(t *testing.T, agent, endpoint string) time.Duration {
	t.Helper()
	var budgets endpointBudgets
	readIntakeYAML(t, filepath.Join(agentDir(t, agent), "rest.yaml"), &budgets)
	for _, server := range budgets.Rest.Servers {
		declared, ok := server.Endpoints[endpoint]
		if !ok || declared.MachineRequest.Timeout == "" {
			continue
		}
		parsed, err := time.ParseDuration(declared.MachineRequest.Timeout)
		if err != nil {
			t.Fatalf("%s endpoint %s timeout %q: %v", agent, endpoint, declared.MachineRequest.Timeout, err)
		}
		return parsed
	}
	t.Fatalf("%s declares no machine_request timeout for endpoint %s", agent, endpoint)
	return 0
}

// TestHopBudgetsContainTheHopTheyWaitOn keeps a request bound from expiring
// while the request it is waiting on is still allowed to run.
//
// This is a different rule from the one agent-core's profile audit enforces.
// That one is about a single agent: every authority an action dispatches under
// has to sit strictly below its own machine's command_timeout, so the machine
// outlives any one operation. It says nothing across agents, because it cannot
// see the far side. The creator's rollout endpoint allowed 15s while the applier
// endpoint it waits on allows 30s -- deliberately widened, because two real
// kubectl reads can exceed ten seconds on a healthy cluster -- so a healthy
// 16-29s read died one hop above the budget written for it, and the applier's
// allowance was unusable (GH-215).
func TestHopBudgetsContainTheHopTheyWaitOn(t *testing.T) {
	for _, hop := range []struct {
		caller, callerEndpoint string
		callee, calleeEndpoint string
	}{
		{"creator", "rollout", "applier", "rollout"},
		{"creator", "state", "applier", "state"},
		{"provisioning-workflow-orchestrator", "rollout", "creator", "rollout"},
		{"provisioning-workflow-orchestrator", "state", "creator", "state"},
	} {
		outer := endpointBudget(t, hop.caller, hop.callerEndpoint)
		inner := endpointBudget(t, hop.callee, hop.calleeEndpoint)
		if outer <= inner {
			t.Errorf(
				"%s/%s allows %s while the %s/%s it waits on allows %s: a call finishing"+
					" inside the far side's budget still fails at this hop, so the far"+
					" side's allowance cannot be used",
				hop.caller, hop.callerEndpoint, outer, hop.callee, hop.calleeEndpoint, inner)
		}
	}
}

// TestRagQueryBudgetCoversItsSequentialReads keeps the query endpoint from
// expiring mid-leg. The query runs two chroma operations in sequence, each
// allowed the chroma client's own limit, so a bound equal to one of them cannot
// cover both and a slow Chroma surfaces as a request timeout rather than as the
// machine's own mapped failure.
func TestRagQueryBudgetCoversItsSequentialReads(t *testing.T) {
	const sequentialChromaReads = 2

	var limits struct {
		Rest struct {
			Limits map[string]struct {
				Timeout string `yaml:"timeout"`
			} `yaml:"limits"`
		} `yaml:"rest"`
	}
	readIntakeYAML(t, filepath.Join(agentDir(t, "rag-server"), "rest.yaml"), &limits)
	chroma, ok := limits.Rest.Limits["local_rag_client"]
	if !ok {
		t.Fatal("the rag-server declares no local_rag_client limits")
	}
	perRead, err := time.ParseDuration(chroma.Timeout)
	if err != nil {
		t.Fatalf("chroma client timeout %q: %v", chroma.Timeout, err)
	}

	query := endpointBudget(t, "rag-server", "query")
	if want := perRead * sequentialChromaReads; query <= want {
		t.Errorf(
			"the query endpoint allows %s while its %d sequential chroma reads may take %s:"+
				" the request expires before the machine can report why",
			query, sequentialChromaReads, want)
	}
}
