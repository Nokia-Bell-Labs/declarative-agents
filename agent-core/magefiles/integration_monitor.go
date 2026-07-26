// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"os"
	"os/exec"
)

// Monitor proves embedded monitor service wiring through cmd/agent.
func (Integration) Monitor() error {
	beginUC("monitor")
	cmd := exec.Command(
		"go", "test", "./cmd/agent", "./internal/tools/rest",
		"-run", "TestMonitorReleaseProfileProof|TestMonitorCLIProfileServesUntilControlExit|TestMonitorProfileUsesEphemeralLoopbackDefault|TestMonitorREST_FactoryUsesLiveMonitorState",
		"-count=1",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("monitor: embedded service proof failed: %w", err)
	}
	fmt.Println("monitor: PASS - embedded service records and serves live state")
	return nil
}
