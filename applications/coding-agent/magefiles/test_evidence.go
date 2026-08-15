// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"fmt"
	"path/filepath"
)

// runTestEvidenceAudit delegates inventory, claim resolution, execution,
// reduction, and reporting to the declarative specification-critic audit profile.
func runTestEvidenceAudit(binary, root, coreRoot, profilesRoot string) error {
	profile := filepath.Join(profilesRoot, "agents", "specification-critic", "audit-profile.yaml")
	if err := runAgentPreflight(binary,
		"--profile", profile,
		"--directory", root,
		"--core-root", coreRoot,
	); err != nil {
		return fmt.Errorf("formal go_test evidence audit failed: %w", err)
	}
	return nil
}
