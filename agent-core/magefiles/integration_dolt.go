// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"os"
	"os/exec"
)

// Dolt proves checkpoint persistence and command-state rehydration against a real
// Dolt SQL server. The test harness launches `dolt sql-server` from a prebuilt
// dolt binary for the duration of the run (no Docker, no manual setup), so this
// target only needs a dolt binary on PATH (or AGENT_CORE_DOLT_BIN).
func (Integration) Dolt() error {
	beginUC("dolt")
	if _, err := exec.LookPath("dolt"); err != nil && os.Getenv("AGENT_CORE_DOLT_BIN") == "" {
		return skipUC("dolt", "no dolt binary on PATH; install dolt (https://docs.dolthub.com/introduction/installation) or set AGENT_CORE_DOLT_BIN")
	}

	cmd := exec.Command(
		"go", "test", "./cmd/agent",
		"-run", "TestDoltCheckpoint|TestDoltCommandStateRehydratesThroughRealAdapter",
		"-count=1",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("dolt: live persistence proof failed: %w", err)
	}
	fmt.Println("dolt: PASS - checkpoints survive adapter and process boundaries")
	return nil
}
