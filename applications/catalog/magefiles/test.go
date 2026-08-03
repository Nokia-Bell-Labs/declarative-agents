// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"

	"github.com/magefile/mage/sh"
)

// Test runs Go unit tests for the catalog.
func Test() error {
	fmt.Println("running go test ./...")
	return sh.Run("go", "test", "./...")
}

// Conformance runs the deterministic per-family profile conformance gate. It
// includes static, protocol, and profile tests and disables live inference even
// when the caller's shell has retained the live-conformance opt-in.
func Conformance() error {
	fmt.Println("running go test ./conformance -count=1")
	return sh.Run("go", "test", "./conformance", "-count=1", "-args", "-live=false")
}

// LiveConformance explicitly enables conformance paths that perform inference
// against the exact Ollama models declared by each test. Dependency checks still
// skip unavailable models; direct go test callers can override the five-minute
// per-run timeout with -args -live=true -live-timeout=<duration>.
func LiveConformance() error {
	fmt.Println("running live model conformance (unavailable declared models will skip)")
	return sh.Run(
		"go", "test", "./conformance", "-count=1", "-args",
		"-live=true", "-live-timeout=5m",
	)
}
