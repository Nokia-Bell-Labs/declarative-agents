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
