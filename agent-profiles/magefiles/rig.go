// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"

	"github.com/magefile/mage/sh"
)

// Rig runs the assembler test rig's self-proof: the assembler discovers the
// reference subject's scenarios, composes each one from real subprocesses —
// twin, subject, validators — and the run must land with happy-path and
// dep-failure passed and the deliberately broken expectation failed, twice in
// a row so ports and state are shown not to leak. It builds the agent binary
// from AGENT_CORE_ROOT (or the sibling ../agent-core checkout) and skips
// without one. Test already covers ./conformance, so this target exists so CI
// and road-map proof lines can invoke the rig explicitly.
func Rig() error {
	fmt.Println("running go test ./conformance -run TestRigSelfProof -count=1")
	return sh.Run("go", "test", "./conformance", "-run", "TestRigSelfProof", "-count=1", "-v")
}
