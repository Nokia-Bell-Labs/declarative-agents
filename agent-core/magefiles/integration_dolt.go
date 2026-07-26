// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"time"
)

const doltIntegrationAddress = "127.0.0.1:3306"

// Dolt proves checkpoint persistence and command-state rehydration against a
// live Dolt SQL server. Run `mage dolt:up` to provide the sidecar.
func (Integration) Dolt() error {
	beginUC("dolt")
	conn, err := net.DialTimeout("tcp", doltIntegrationAddress, 3*time.Second)
	if err != nil {
		return skipUC("dolt", fmt.Sprintf(
			"no Dolt SQL server at %s; run `mage dolt:up`: %v",
			doltIntegrationAddress, err,
		))
	}
	_ = conn.Close()

	cmd := exec.Command(
		"go", "test", "./cmd/agent",
		"-run", "TestDoltCheckpointSuspendResumeRoundTrip|TestDoltCheckpointSuspendResumeAcrossProcesses|TestDoltCommandStateRehydratesThroughRealAdapter",
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
