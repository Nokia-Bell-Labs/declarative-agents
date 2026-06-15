// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"os"
	"os/exec"
)

// Uc004 runs rel04.0: embedded monitor profile proof through cmd/agent wiring.
func (Integration) Uc004() error {
	cmd := exec.Command(
		"go", "test", "./cmd/agent", "./internal/tools/rest",
		"-run", "TestMonitorReleaseProfileProof|TestMonitorREST_FactoryUsesLiveMonitorState",
		"-count=1",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("uc004: monitor release proof failed: %w", err)
	}
	fmt.Println("uc004: PASS - monitor profile records and serves live embedded state")
	return nil
}
