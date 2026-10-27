// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

// Package version holds the release the words were published at. The
// application is a nested module of the repository, so the repository's
// release tag never reaches the module's build info; the Publisher stamps
// it here with -ldflags instead, and everything that names the running
// release reads it from here (GH-18).
package version

import (
	"regexp"
	"runtime/debug"
)

// Release is stamped by the Publisher: -X <this package>.Release=<tag>.
var Release string

// Runtime is the declarative-agents release the binary was built against,
// stamped by the Publisher beside Release (GH-2755): the release a new
// corpus pins, which is Release itself when the words build beside the
// runtime they ship with, and the pinned release when they build against
// one.
var Runtime string

var releaseTag = regexp.MustCompile(`^v0\.\d{8}\.\d+$`)

// Current is the release this binary was published at, else the module
// version it was built at, else "dev".
func Current() string {
	if Release != "" {
		return Release
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return "dev"
}

// IsRelease reports whether v is a published release tag, v0.YYYYMMDD.N.
func IsRelease(v string) bool { return releaseTag.MatchString(v) }
