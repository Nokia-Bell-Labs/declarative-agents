// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"strings"
)

type integrationTarget struct {
	name       string
	fn         func() error
	sharedKind bool
}

func integrationTargets(i Integration) []integrationTarget {
	return []integrationTarget{
		{name: "chatbot", fn: i.Chatbot},
		{name: "chroma", fn: i.Chroma},
		{name: "ragServer", fn: i.RagServer},
		{name: "controlPlane", fn: i.ControlPlane},
		{name: "embeddingExclusion", fn: i.EmbeddingExclusion},
		{name: "observer", fn: i.Observer},
		{name: "policyProof", fn: i.PolicyProof},
		{name: "rig", fn: i.Rig},
		{name: "helmSmoke", fn: i.HelmSmoke, sharedKind: true},
		{name: "helmSwap", fn: i.HelmSwap, sharedKind: true},
		{name: "helmLLMTier", fn: i.HelmLLMTier, sharedKind: true},
		{name: "applier", fn: i.Applier},
		{name: "applierLive", fn: i.ApplierLive, sharedKind: true},
	}
}

// SharedSmokeSwap runs the two namespace-isolated scenarios used to measure
// and verify shared-session data-plane readiness without the unrelated local,
// policy, LLM-tier, and applier targets.
func (i Integration) SharedSmokeSwap() error {
	return runSharedKindTargets([]integrationTarget{
		{name: "helmSmoke", fn: i.HelmSmoke, sharedKind: true},
		{name: "helmSwap", fn: i.HelmSwap, sharedKind: true},
	})
}

// SharedSmokeApplier prepares the default-CNI session through the smoke
// scenario, then measures applierLive on the same clean cluster. It is the
// focused repeatable gate for the warm applier target's release budget.
func (i Integration) SharedSmokeApplier() error {
	return runSharedKindTargets([]integrationTarget{
		{name: "helmSmoke", fn: i.HelmSmoke, sharedKind: true},
		{name: "applierLive", fn: i.ApplierLive, sharedKind: true},
	})
}

// SharedApplierBenchmark prepares one smoke-proven session and runs the warm
// applier proof three times, matching the performance acceptance measurement
// without repeating unrelated application targets or cluster setup.
func (i Integration) SharedApplierBenchmark() error {
	return runSharedKindTargets([]integrationTarget{
		{name: "helmSmoke", fn: i.HelmSmoke, sharedKind: true},
		{name: "applierLive-1", fn: i.ApplierLive, sharedKind: true},
		{name: "applierLive-2", fn: i.ApplierLive, sharedKind: true},
		{name: "applierLive-3", fn: i.ApplierLive, sharedKind: true},
	})
}

// SharedLLMBenchmark prepares one smoke-proven session, populates the
// identity-keyed model cache once, then records three warm LLM-tier runs.
func (i Integration) SharedLLMBenchmark() error {
	return runSharedKindTargets([]integrationTarget{
		{name: "helmSmoke", fn: i.HelmSmoke, sharedKind: true},
		{name: "helmLLMTier-bootstrap", fn: i.HelmLLMTier, sharedKind: true},
		{name: "helmLLMTier-warm-1", fn: i.HelmLLMTier, sharedKind: true},
		{name: "helmLLMTier-warm-2", fn: i.HelmLLMTier, sharedKind: true},
		{name: "helmLLMTier-warm-3", fn: i.HelmLLMTier, sharedKind: true},
	})
}

func runSharedKindTargets(targets []integrationTarget) error {
	session := newIntegrationKindSession(integrationKindSessionRoot())
	deactivate, err := activateIntegrationKindSession(session)
	if err != nil {
		return err
	}
	defer deactivate()
	defer session.close()
	for _, target := range targets {
		if err := session.runTarget(target.name, target.fn); err != nil {
			return err
		}
	}
	return nil
}

// All runs every integration target this application owns and prints a
// pass/fail/skip summary, returning an error when any target fails. Each target
// self-skips (returns nil after printing SKIP) when an optional live
// prerequisite -- Docker, kind, Helm, or a local model server -- is missing, so
// the aggregate is portable to a machine without them while still exercising
// and gating every runnable target on capable hosts. This aggregate is what
// lets the released application participate in the repository release gate
// rather than being tagged without its own integration evidence (GH-1343).
func (i Integration) All() error {
	session := newIntegrationKindSession(integrationKindSessionRoot())
	deactivate, err := activateIntegrationKindSession(session)
	if err != nil {
		return err
	}
	defer deactivate()

	var results []string
	failed := 0
	for _, t := range integrationTargets(i) {
		fmt.Printf("\n=== %s ===\n", t.name)
		run := t.fn
		if t.sharedKind {
			run = func() error { return session.runTarget(t.name, t.fn) }
		}
		if err := run(); err != nil {
			failed++
			results = append(results, fmt.Sprintf("  FAIL  %s  %v", t.name, err))
			continue
		}
		results = append(results, fmt.Sprintf("  PASS  %s", t.name))
	}
	session.close()

	fmt.Printf("\n%s\n", strings.Repeat("─", 40))
	for _, r := range results {
		fmt.Println(r)
	}
	fmt.Printf("%s\n", strings.Repeat("─", 40))
	if failed > 0 {
		return fmt.Errorf("%d integration target(s) failed", failed)
	}
	return nil
}
