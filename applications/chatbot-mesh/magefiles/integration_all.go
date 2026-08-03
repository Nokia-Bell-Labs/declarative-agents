// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"strings"
)

// All runs every integration target this application owns and prints a
// pass/fail/skip summary, returning an error when any target fails. Each target
// self-skips (returns nil after printing SKIP) when an optional live
// prerequisite -- Docker, kind, Helm, or a local model server -- is missing, so
// the aggregate is portable to a machine without them while still exercising
// and gating every runnable target on capable hosts. This aggregate is what
// lets the released application participate in the repository release gate
// rather than being tagged without its own integration evidence (GH-1343).
func (i Integration) All() error {
	targets := []struct {
		name string
		fn   func() error
	}{
		{"chatbot", i.Chatbot},
		{"chroma", i.Chroma},
		{"ragServer", i.RagServer},
		{"controlPlane", i.ControlPlane},
		{"embeddingExclusion", i.EmbeddingExclusion},
		{"observer", i.Observer},
		{"policyProof", i.PolicyProof},
		{"rig", i.Rig},
		{"helmSmoke", i.HelmSmoke},
		{"helmSwap", i.HelmSwap},
		{"helmLLMTier", i.HelmLLMTier},
		{"applier", i.Applier},
		{"applierLive", i.ApplierLive},
	}

	var results []string
	failed := 0
	for _, t := range targets {
		fmt.Printf("\n=== %s ===\n", t.name)
		if err := t.fn(); err != nil {
			failed++
			results = append(results, fmt.Sprintf("  FAIL  %s  %v", t.name, err))
			continue
		}
		results = append(results, fmt.Sprintf("  PASS  %s", t.name))
	}

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
