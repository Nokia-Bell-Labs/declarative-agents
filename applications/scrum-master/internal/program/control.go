// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package program

import (
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/checkout"
)

// ControlFor is the program repository whose control files a word reading
// the policy applies, run from the checkout at dir (GH-46): control when a
// --control (or another flag the word takes for it) names one; else the
// system repository, found as the tracker's routing finds it, so a team
// repository's merge is held to the system's policy; else fallback, which
// is empty for the defaults.
func ControlFor(control, dir, fallback string) string {
	if control != "" {
		return control
	}
	if system := SystemControl(dir); system != "" {
		return system
	}
	return fallback
}

// SystemControl is the system repository of the program the checkout at
// dir belongs to: the checkout holding the trackers document that
// checkout.FindTrackers finds there or beside it, named by its origin's
// project path, or by its directory when it has no origin, as a project's
// checkout is the sibling named for the project's last segment. Empty when
// no checkout holds one.
func SystemControl(dir string) string {
	if dir == "" {
		dir = "."
	}
	file, err := checkout.FindTrackers(dir)
	if err != nil || file == "" {
		return ""
	}
	root := filepath.Dir(filepath.Dir(file))
	// As configured, not as url.insteadOf expands it, so a clone that
	// rewrites its origin still names the system by its project (GH-105).
	out, err := exec.Command("git", "-C", root, "config", "--get", "remote.origin.url").Output()
	if url := strings.TrimSpace(string(out)); err == nil && url != "" {
		if project := ProjectPath(url); filepath.Base(project) == filepath.Base(root) {
			return project
		}
	}
	return filepath.Base(root)
}
