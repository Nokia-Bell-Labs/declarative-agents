// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"fmt"
	"strings"
)

// All runs every integration target this application owns and prints a
// pass/fail/skip summary, returning an error when any target fails. Composite
// targets replace their component targets in this aggregate so release evidence
// is not generated repeatedly; executorLive, plannerDelegation, and criticGate
// remain independently addressable. Each aggregate target self-skips when an
// optional Docker, kind, or Helm prerequisite is missing.
func (i Integration) All() error {
	targets := []struct {
		name string
		fn   func() error
	}{
		{"servingHealth", i.ServingHealth},
		{"servingRemote", i.ServingRemote},
		{"helmSmoke", i.HelmSmoke},
		{"codingLoop", i.CodingLoop},
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
