// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"path/filepath"
	"testing"
)

// The persistent ingress spool is this application's own; the shared helper
// captures whatever path it is given (GH-2290).
func TestDiagnoseTraceSpool(t *testing.T) {
	got := diagnoseTraceSpool("/app")
	want := filepath.Join("/app", "observability", ".run", "spool", "traces", "collector.ndjson")
	if got != want {
		t.Errorf("trace spool = %s, want %s", got, want)
	}
}
