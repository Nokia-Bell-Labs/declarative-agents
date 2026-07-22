// Copyright (c) 2026 Nokia. All rights reserved.

// Package profiles resolves an external agent-profiles checkout.
//
// agent-profiles owns agent programs; agent-core owns the runtime that executes
// them (srd034 R1, R2). Nothing in agent-core may assume a profile tree exists,
// so callers that want one -- tests that exercise the runtime end to end
// against a real profile, and tooling that launches the binary -- ask here and
// handle Absent rather than hard-coding a sibling path (srd034 R3.3, R3.4).
//
// This mirrors the policy agent-profiles applies in the other direction for
// AGENT_CORE_ROOT: an explicitly configured root wins and must be usable, an
// absent prerequisite is reported rather than fatal, and a configured but
// unusable root is an error the caller should surface (GH-584, GH-512).
package profiles

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RootEnv names an explicit agent-profiles checkout. It selects where the
// checkout is, not what a profile means; callers still pass explicit file paths
// derived from the resolved root (srd034 R3.3).
const RootEnv = "AGENT_PROFILES_ROOT"

// Outcome is how the prerequisite resolved.
type Outcome int

const (
	// Found means Path holds a usable agent-profiles checkout.
	Found Outcome = iota
	// Absent means no checkout was configured or discovered.
	Absent
	// Invalid means a root was configured explicitly but does not hold profiles.
	Invalid
)

// Resolution is the resolved prerequisite. Source records how Path was chosen
// so a diagnostic can tell a configured root from a discovered one.
type Resolution struct {
	Path    string
	Source  string
	Outcome Outcome
}

// Reason renders a diagnostic for a resolution that is not Found. It returns
// the empty string when the prerequisite resolved (srd034 R3.4).
func (r Resolution) Reason() string {
	switch r.Outcome {
	case Invalid:
		return fmt.Sprintf("%s is set to %s, which holds no agent profiles", RootEnv, r.Path)
	case Absent:
		return fmt.Sprintf("no agent-profiles checkout found; clone it beside this repository, nest it under ./agent-profiles, or set %s", RootEnv)
	default:
		return ""
	}
}

// Resolve applies the prerequisite policy to an explicitly configured root and
// an ordered list of discovery candidates. It reads no process state, so the
// policy is testable on its own.
func Resolve(configured string, candidates ...string) Resolution {
	if trimmed := strings.TrimSpace(configured); trimmed != "" {
		if root := normalize(trimmed); root != "" {
			return Resolution{Path: root, Source: RootEnv, Outcome: Found}
		}
		return Resolution{Path: trimmed, Source: RootEnv, Outcome: Invalid}
	}
	for _, candidate := range candidates {
		if root := normalize(candidate); root != "" {
			return Resolution{Path: root, Source: "discovered checkout", Outcome: Found}
		}
	}
	return Resolution{Outcome: Absent}
}

// ResolveFrom resolves against the standard candidates for a module root: a
// sibling agent-profiles checkout, then one nested inside the module.
func ResolveFrom(moduleRoot string) Resolution {
	return Resolve(
		os.Getenv(RootEnv),
		filepath.Join(filepath.Dir(moduleRoot), "agent-profiles"),
		filepath.Join(moduleRoot, "agent-profiles"),
	)
}

// AgentsRoot is the directory holding shipped agent profile directories.
func (r Resolution) AgentsRoot() string {
	return filepath.Join(r.Path, "agents")
}

// ConformanceRoot is the directory holding profile-owned conformance fixtures.
func (r Resolution) ConformanceRoot() string {
	return filepath.Join(r.Path, "testdata", "conformance")
}

// normalize accepts either a repository root or the agents/ directory inside
// one, and returns the repository root. A candidate that holds neither is not a
// checkout.
func normalize(candidate string) string {
	if candidate == "" {
		return ""
	}
	if holdsProfiles(filepath.Join(candidate, "agents")) {
		return candidate
	}
	if filepath.Base(candidate) == "agents" && holdsProfiles(candidate) {
		return filepath.Dir(candidate)
	}
	return ""
}

// holdsProfiles reports whether agentsDir carries shipped profiles. Two are
// checked so a partial checkout of one profile does not read as a full one.
func holdsProfiles(agentsDir string) bool {
	found := 0
	for _, name := range []string{"executor", "critic", "jurist", "monitor"} {
		if isFile(filepath.Join(agentsDir, name, "profile.yaml")) {
			found++
		}
	}
	return found >= 2
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
