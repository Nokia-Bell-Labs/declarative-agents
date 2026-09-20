// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"fmt"
	"path/filepath"
)

// runCorpusAudit runs the catalog's specification-critic corpus-audit profile
// over this module's own documents. The profile binds docs/corpus-charter.yaml
// relative to --directory, so the coding-agent's charter decides which checks
// gate here (GH-2346). The agent exits non-zero when its machine reaches the
// Failed terminal, which for this profile means the charter's checks reported
// an error-level finding.
func runCorpusAudit(binary, root, coreRoot, profilesRoot string) error {
	profile := filepath.Join(profilesRoot, "agents", "specification-critic", "corpus-audit-profile.yaml")
	if err := runAgentPreflight(binary,
		"--profile", profile,
		"--directory", root,
		"--core-root", coreRoot,
	); err != nil {
		return fmt.Errorf("specification corpus audit failed under %s: %w", root, err)
	}
	return nil
}
