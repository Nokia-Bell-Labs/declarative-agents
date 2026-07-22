// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// profileSmokeRunner runs one preflight command and returns its combined output.
// Injected so the smoke can be tested without building or executing an agent.
type profileSmokeRunner func(binary string, args ...string) ([]byte, error)

func defaultSmokeRun(binary string, args ...string) ([]byte, error) {
	return exec.Command(binary, args...).CombinedOutput()
}

// discoverAuditProfiles returns every profile the audit governs: the shipped
// agents plus the conformance grammar fixtures. The fixtures live under
// testdata/conformance rather than agents/ (they are not roles, GH-328) but are
// still profiles, and the directory is optional so a synthetic root with only
// agents/ still audits.
func discoverAuditProfiles(root string) ([]string, error) {
	profiles, err := discoverProfiles(filepath.Join(root, "agents"))
	if err != nil {
		return nil, err
	}
	if len(profiles) == 0 {
		return nil, fmt.Errorf("no profile-shaped YAML files found under agents")
	}
	conformanceDir := filepath.Join(root, "testdata", "conformance")
	if _, statErr := os.Stat(conformanceDir); statErr == nil {
		fixtures, err := discoverProfiles(conformanceDir)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, fixtures...)
	}
	return profiles, nil
}

// BootSmoke loads every audited profile through the agent runtime's own startup
// path and fails on the first profile that cannot boot.
func BootSmoke() error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	coreRoot := envOrDefault(agentCoreRootEnv, filepath.Join(filepath.Dir(root), "agent-core"))
	return bootSmoke(root, coreRoot)
}

// bootSmoke builds the agent from coreRoot and preflights every audited profile.
func bootSmoke(root, coreRoot string) error {
	profiles, err := discoverAuditProfiles(root)
	if err != nil {
		return err
	}
	binary, err := buildIntegrationAgent(coreRoot)
	if err != nil {
		return err
	}
	return bootSmokeProfiles(defaultSmokeRun, binary, coreRoot, profiles)
}

// bootSmokeProfiles runs `agent --validate-config` over each profile, which is
// the same load path the runtime takes at startup: it parses the profile,
// machine, and REST definitions with strict field checking (GH-486) and runs
// ValidateToolEmits and ValidateReceiptContracts over the selected ToolDefs
// (GH-494), then exits without binding a listener or running the machine.
//
// The static wiring checks in validateProfiles read the YAML shape only, so they
// pass for a profile the runtime rejects. This closes that gap: a mis-declared
// lifecycle word or an unknown REST field fails the audit instead of surfacing
// the first time an agent actually starts (GH-614).
//
// Every profile is attempted before reporting, so one broken declaration does not
// hide the rest.
func bootSmokeProfiles(run profileSmokeRunner, binary, coreRoot string, profiles []string) error {
	var failures []string
	for _, profile := range profiles {
		out, err := run(binary, "--validate-config", "--profile", profile, "--core-root", coreRoot)
		if err == nil {
			continue
		}
		detail := strings.TrimSpace(string(out))
		if detail == "" {
			detail = err.Error()
		}
		failures = append(failures, fmt.Sprintf("  %s: %s", profile, detail))
	}
	if len(failures) > 0 {
		return fmt.Errorf("profile boot smoke failed for %d of %d profile(s):\n%s",
			len(failures), len(profiles), strings.Join(failures, "\n"))
	}
	fmt.Printf("boot smoke passed for %d profiles against %s\n", len(profiles), coreRoot)
	return nil
}
