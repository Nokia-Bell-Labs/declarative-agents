// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"os"
	"os/exec"
)

// BenchEvaluator executes the shipped bench wrapper through the behavioral
// conformance case. That case starts the real agent-core runtime, sends a human
// action through the bench REST route, and observes the profile dispatching the
// configured critic child.
func (Integration) BenchEvaluator() error {
	profilesRoot, err := catalogOwnerRoot("catalog integration:benchEvaluator")
	if err != nil {
		return err
	}
	if err := requireProfilePaths(profilesRoot, "agents/bench/profile.yaml", "agents/critic/profile.yaml"); err != nil {
		return err
	}
	cmd := exec.Command("go", "test", "./conformance", "-run", "^TestBenchConformance$", "-count=1", "-v")
	cmd.Dir = profilesRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run shipped bench behavioral conformance: %w", err)
	}
	fmt.Println("integration:benchEvaluator PASS - shipped bench accepted a human action and dispatched critic")
	return nil
}
