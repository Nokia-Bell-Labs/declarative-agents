// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// renderStagedChart stages the chart with its profile programs and renders it
// with default values (collector.implementation=agent), returning the manifest.
func renderStagedChart(t *testing.T, args ...string) string {
	t.Helper()
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	chartDir := findChartDir(t)
	staged, cleanup, err := stageSmokeChart(chartDir, filepath.Dir(chartDir))
	if err != nil {
		t.Fatalf("stage chart: %v", err)
	}
	defer cleanup()
	full := append([]string{"template", "relx", staged, "--namespace", "nsy"}, args...)
	out, err := exec.Command("helm", full...).CombinedOutput()
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	return string(out)
}

// TestCollectorServesUI proves the canonical collector serves the staged trace
// UI from a collector-only ConfigMap mounted at /collector-ui, with the working
// directory set so the literal ui/dist root resolves and the metrics spool
// pointed at the writable work volume (srd020 R7, GH-1256, GH-1228).
func TestCollectorServesUI(t *testing.T) {
	render := renderStagedChart(t)
	for _, want := range []string{
		"name: relx-chatbot-mesh-collector-ui",
		"ui__dist__index.html",
		"path: ui/dist/index.html",
		"workingDir: /collector-ui",
		"mountPath: /collector-ui",
		"COLLECTOR_METRICS_SPOOL_PATH",
		"/work/metrics/collector.ndjson",
	} {
		if !strings.Contains(render, want) {
			t.Errorf("collector UI render missing %q", want)
		}
	}
	// The chatbot legitimately carries CHATBOT_UI_ROOT; the collector must add no
	// UI root variable of its own (GH-1228).
	if strings.Contains(render, "COLLECTOR_UI_ROOT") {
		t.Error("collector render unexpectedly sets COLLECTOR_UI_ROOT (GH-1228)")
	}
}

// TestCollectorUIStaysOutOfSharedProfiles proves the collector UI bundle does
// not enter the shared profiles ConfigMap that every pod mounts; it belongs to
// the collector-only ConfigMap so the shared object stays under its size limit.
func TestCollectorUIStaysOutOfSharedProfiles(t *testing.T) {
	for _, key := range renderedProfileKeys(t) {
		if strings.Contains(key, "collector__ui") || strings.Contains(key, "collector__agents__collector__ui") {
			t.Errorf("shared profiles ConfigMap carries collector UI key %s; it belongs on the collector-only ConfigMap", key)
		}
	}
}
