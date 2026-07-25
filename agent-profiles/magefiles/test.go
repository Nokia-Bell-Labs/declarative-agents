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

// Conformance runs the deterministic per-family profile conformance gate. It
// includes static, protocol, and profile tests; live model inference remains
// skipped unless AGENT_PROFILES_LIVE_CONFORMANCE=1 is explicitly set.
func Conformance() error {
	fmt.Println("running go test ./conformance -count=1")
	return sh.Run("go", "test", "./conformance", "-count=1")
}

// LiveConformance explicitly enables conformance paths that perform inference
// against the exact Ollama models declared by each test. Dependency checks still
// skip unavailable models; AGENT_PROFILES_LIVE_TIMEOUT can override their
// default per-run timeout using Go duration syntax.
func LiveConformance() error {
	fmt.Println("running live model conformance (unavailable declared models will skip)")
	return sh.RunWith(
		map[string]string{"AGENT_PROFILES_LIVE_CONFORMANCE": "1"},
		"go", "test", "./conformance", "-count=1",
	)
}
