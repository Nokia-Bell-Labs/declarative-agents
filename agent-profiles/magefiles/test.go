// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"

	"github.com/magefile/mage/sh"
)

// Test runs Go unit tests for agent-profiles.
func Test() error {
	fmt.Println("running go test ./...")
	return sh.Run("go", "test", "./...")
}

// Conformance runs the per-family profile conformance tests. They build the
// agent binary from AGENT_CORE_ROOT and assert on OpenTelemetry trace output;
// without AGENT_CORE_ROOT set they skip. Test already covers ./..., so this
// target exists so CI and road-map proof lines can invoke the suite explicitly.
func Conformance() error {
	fmt.Println("running go test ./conformance -count=1")
	return sh.Run("go", "test", "./conformance", "-count=1")
}
