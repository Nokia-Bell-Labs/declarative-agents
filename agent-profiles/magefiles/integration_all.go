// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// All runs every profile-owned integration tracer target and prints a
// pass/fail/skip summary. Targets that build the agent-core binary are skipped
// (not failed) when no agent-core checkout is reachable, so the aggregate stays
// usable in a profiles-only checkout.
func (i Integration) All() error {
	profilesRoot, err := os.Getwd()
	if err != nil {
		return err
	}
	coreRoot := envOrDefault(agentCoreRootEnv, filepath.Join(filepath.Dir(profilesRoot), "agent-core"))
	coreAvailable := agentCoreCheckoutAvailable(coreRoot)

	targets := []struct {
		name      string
		fn        func() error
		needsCore bool
	}{
		{"documentationCurator", i.DocumentationCurator, true},
		{"evaluatorGenerator", i.EvaluatorGenerator, true},
		{"plannerGenerator", i.PlannerGenerator, false},
		{"benchEvaluator", i.BenchEvaluator, false},
		{"monitorControl", i.MonitorControl, false},
		{"chroma", i.Chroma, true},
	}

	var passed, failed, skipped int
	results := make([]string, 0, len(targets))

	for _, t := range targets {
		fmt.Printf("\n=== %s ===\n", t.name)
		if t.needsCore && !coreAvailable {
			skipped++
			reason := fmt.Sprintf("agent-core checkout not found at %s (set %s)", coreRoot, agentCoreRootEnv)
			fmt.Printf("SKIP %s: %s\n", t.name, reason)
			results = append(results, fmt.Sprintf("  SKIP  %s", t.name))
			continue
		}
		switch err := t.fn(); {
		case err != nil:
			failed++
			results = append(results, fmt.Sprintf("  FAIL  %s  %v", t.name, err))
		default:
			passed++
			results = append(results, fmt.Sprintf("  PASS  %s", t.name))
		}
	}

	fmt.Printf("\n%s\n", strings.Repeat("─", 40))
	for _, r := range results {
		fmt.Println(r)
	}
	fmt.Printf("%s\n", strings.Repeat("─", 40))
	fmt.Printf("Total: %d passed, %d failed, %d skipped\n", passed, failed, skipped)

	if failed > 0 {
		return fmt.Errorf("%d integration target(s) failed", failed)
	}
	return nil
}

// agentCoreCheckoutAvailable reports whether coreRoot looks like an agent-core
// module checkout that buildIntegrationAgent can compile from.
func agentCoreCheckoutAvailable(coreRoot string) bool {
	info, err := os.Stat(filepath.Join(coreRoot, "go.mod"))
	return err == nil && !info.IsDir()
}
