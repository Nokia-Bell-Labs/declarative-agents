// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"strings"
	"testing"
)

// TestHelmCollectorServesUI proves the collector serves the staged trace UI:
// ui/dist reaches the container through the profiles ConfigMap projection, and
// the container working directory is the profile directory so the literal
// ui/dist root resolves without an environment variable (srd020 R7, GH-1255).
func TestHelmCollectorServesUI(t *testing.T) {
	chart := preparedTestChart(t)
	render := helmTemplate(t, chart)
	for _, want := range []string{
		"workingDir: /profiles/agents/collector",
		"agents__collector__ui__dist__index.html",
		"path: agents/collector/ui/dist/index.html",
	} {
		if !strings.Contains(render, want) {
			t.Errorf("collector UI render missing %q", want)
		}
	}
	if strings.Contains(render, "_UI_ROOT") {
		t.Error("collector render unexpectedly sets a UI root environment variable (GH-1228)")
	}
}
