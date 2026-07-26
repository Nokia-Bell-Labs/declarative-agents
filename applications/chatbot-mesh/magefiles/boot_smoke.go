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

// meshProfiles returns the shipped mesh agent profiles under agents/.
func meshProfiles(root string) ([]string, error) {
	agentsDir := filepath.Join(root, "agents")
	var profiles []string
	err := filepath.WalkDir(agentsDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && entry.Name() == "profile.yaml" {
			profiles = append(profiles, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(profiles) == 0 {
		return nil, fmt.Errorf("no agent profiles found under %s", agentsDir)
	}
	return profiles, nil
}

// bootSmokeProfiles runs `agent --validate-config` over each mesh profile, which
// is the same load path the runtime takes at startup: it parses the profile,
// machine, and REST definitions with strict field checking (GH-486) and runs
// ValidateToolEmits and ValidateReceiptContracts over the selected ToolDefs
// (GH-494), then exits without binding a listener or running the machine.
//
// The jurist audit validates the specification corpus, not whether an agent can
// start, so a mis-declared lifecycle word or an unknown REST field passed the
// gate and only surfaced when the mesh was deployed. This closes that gap
// (GH-614).
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
		return fmt.Errorf("mesh boot smoke failed for %d of %d profile(s):\n%s",
			len(failures), len(profiles), strings.Join(failures, "\n"))
	}
	fmt.Printf("boot smoke passed for %d mesh profiles against %s\n", len(profiles), coreRoot)
	return nil
}
