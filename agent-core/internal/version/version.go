// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

// Package version carries build-time identity for agent-core binaries.
// The variables are populated via -ldflags "-X"; the defaults identify
// an uninjected development build.
//
// Version is the identity recorded in checkpoints and traces so
// check_agent_version can compare equal values across rebuilds of the
// same tag. String() is the human-readable CLI --version text and
// includes commit and date when they were injected.
package version

const unknown = "unknown"

var (
	Version = "v0.0.0-dev"
	Commit  = unknown
	Date    = unknown
)

// String returns "vX.Y.Z (commit abcdef0, built 2026-08-20)".
// Uninjected builds degrade to Version alone, matching a plain go build.
func String() string {
	return format(Version, Commit, Date)
}

func format(ver, commit, date string) string {
	switch {
	case commit == unknown && date == unknown:
		return ver
	case commit == unknown:
		return ver + " (built " + date + ")"
	case date == unknown:
		return ver + " (commit " + commit + ")"
	default:
		return ver + " (commit " + commit + ", built " + date + ")"
	}
}
