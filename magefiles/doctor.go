// Copyright (c) 2026 Nokia. All rights reserved.

package main

// Doctor reports whether the repository-wide checks can run, without mutating
// anything.
//
// It covers the lint toolchain only. The cluster toolchain -- docker, kind, helm,
// kubectl, and Docker Desktop resources -- is reported by each application's own
// doctor through kindrig, and golangci-lint deliberately stays out of that set:
// kindrig.Doctor gates cluster work, including demo:up, and needing a linter
// installed to bring up a demo would be the wrong coupling (GH-1479).
func Doctor() error {
	return reportGolangciLint()
}
