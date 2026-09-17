// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/helmlib"
)

// TestMain vendors the shared agent-services library chart into this
// application's chart before any test renders it. Helm resolves a dependency
// only from the chart's own charts/ directory, and the vendored copy is
// generated rather than tracked, so a clean checkout has none until a target
// that packages or prepares the chart runs. The copy is a directory copy, so
// doing it here costs nothing and keeps `mage test` and `mage audit` working
// with no prior target (GH-2175).
func TestMain(m *testing.M) {
	// Resolved from this file rather than the working directory: a test that
	// chdirs into a temp tree re-executes this binary as a fixture child, and
	// that child runs TestMain too.
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		fmt.Fprintln(os.Stderr, "resolve the magefiles directory")
		os.Exit(1)
	}
	application := filepath.Dir(filepath.Dir(file))
	if err := helmlib.Vendor(filepath.Join(application, "..", ".."), filepath.Join(application, "helm")); err != nil {
		fmt.Fprintf(os.Stderr, "vendor the agent-services library chart: %v\n", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}
