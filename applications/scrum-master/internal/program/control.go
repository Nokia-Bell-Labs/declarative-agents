// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package program

import (
	"path/filepath"

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
// project path whatever its folder is called, as a worktree pop opens is
// named for its branch (GH-2752); an origin that is a local path names it
// when its last segment is the main checkout's name, and with neither the
// main checkout's name does, as a project's checkout is the sibling named
// for the project's last segment. Empty when no checkout holds one.
func SystemControl(dir string) string {
	if dir == "" {
		dir = "."
	}
	file, err := checkout.FindTrackers(dir)
	if err != nil || file == "" {
		return ""
	}
	root := filepath.Dir(filepath.Dir(file))
	name := filepath.Base(checkout.MainCheckout(root))
	// As configured, not as url.insteadOf expands it, so a clone that
	// rewrites its origin still names the system by its project (GH-105).
	switch url := checkout.Origin(root); {
	case checkout.IsForge(url):
		return ProjectPath(url)
	case url != "":
		if project := ProjectPath(url); filepath.Base(project) == name {
			return project
		}
	}
	return name
}
