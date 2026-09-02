// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// applierExecBlock returns the local wrapper's exec-declarations value
// from a rendered profiles ConfigMap, up to the next ConfigMap key.
func applierExecBlock(rendered string) string {
	const marker = "applications__chatbot-mesh__applier__exec-declarations.yaml:"
	i := strings.Index(rendered, marker)
	if i < 0 {
		return ""
	}
	rest := rendered[i:]
	if j := strings.Index(rest, "applications__chatbot-mesh__applier__profile.yaml:"); j > 0 {
		return rest[:j]
	}
	return rest
}

// TestApplierExecDeclarationsRenderReleaseCoordinates proves the applier's
// helm/kubectl exec args target the installed release, namespace, and chatbot
// Deployment rather than a baked chatbot-mesh/default (GH-484). It stages the
// chart through the production packaging path and renders under a non-default
// release name and namespace.
func TestApplierExecDeclarationsRenderReleaseCoordinates(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	chartDir := findChartDir(t)
	profilesRoot := filepath.Dir(chartDir)

	staged, cleanup, err := stageSmokeChart(chartDir, profilesRoot)
	if err != nil {
		t.Fatalf("stage chart: %v", err)
	}
	defer cleanup()

	out, err := exec.Command("helm", "template", "relx", staged, "--namespace", "nsy").CombinedOutput()
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	block := applierExecBlock(string(out))
	if block == "" {
		t.Fatal("applier exec-declarations key not found in rendered ConfigMap")
	}

	// The release-derived coordinates must appear.
	wantPresent := []string{
		"upgrade, relx,",
		"--namespace, nsy,",
		"rollback, relx]",
		"get, values, relx,",
		"deployment/relx-chatbot-mesh-chatbot,",
	}
	for _, w := range wantPresent {
		if !strings.Contains(block, w) {
			t.Errorf("applier exec args missing %q under release relx/nsy", w)
		}
	}
	// The baked defaults must be gone.
	wantAbsent := []string{
		"upgrade, chatbot-mesh,",
		"namespace, default,",
		"rollback, chatbot-mesh]",
		"get, values, chatbot-mesh,",
		"deployment/chatbot-mesh-chatbot,",
	}
	for _, w := range wantAbsent {
		if strings.Contains(block, w) {
			t.Errorf("applier exec args still carry baked default %q", w)
		}
	}
}

// argvFields splits a rendered `args: [...]` line into its comma-delimited
// fields. Fields carrying embedded commas (the kubectl go-template) split into
// fragments, which is harmless: the callers test whole-field equality against
// coordinate tokens no fragment can match.
func argvFields(line string) []string {
	open := strings.Index(line, "[")
	closing := strings.LastIndex(line, "]")
	if open < 0 || closing <= open {
		return nil
	}
	raw := strings.Split(line[open+1:closing], ",")
	fields := make([]string, 0, len(raw))
	for _, f := range raw {
		fields = append(fields, strings.Trim(strings.TrimSpace(f), `"'`))
	}
	return fields
}

// TestApplierExecArgsCarryNoBakedReleaseCoordinate guards the whole GH-484
// class rather than the four commands that motivated it: every exec word's
// argv is scanned, so an exec word added later whose release coordinate the
// packaging rewrite does not cover fails here instead of silently reading
// another release's state (GH-217).
//
// The check is whole-field equality, not substring: the rewritten Deployment
// token legitimately contains the chart name (deployment/relx-chatbot-mesh-chatbot),
// while a surviving release argument is always a bare `chatbot-mesh` field.
func TestApplierExecArgsCarryNoBakedReleaseCoordinate(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	chartDir := findChartDir(t)
	staged, cleanup, err := stageSmokeChart(chartDir, filepath.Dir(chartDir))
	if err != nil {
		t.Fatalf("stage chart: %v", err)
	}
	defer cleanup()

	out, err := exec.Command("helm", "template", "relx", staged, "--namespace", "nsy").CombinedOutput()
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	block := applierExecBlock(string(out))
	if block == "" {
		t.Fatal("applier exec-declarations key not found in rendered ConfigMap")
	}

	var scanned int
	for _, line := range strings.Split(block, "\n") {
		if !strings.Contains(line, "args: [") {
			continue
		}
		scanned++
		fields := argvFields(line)
		for i, f := range fields {
			if f == "chatbot-mesh" {
				t.Errorf("exec argv keeps the placeholder release as a bare argument: %s", strings.TrimSpace(line))
			}
			if f == "--namespace" && i+1 < len(fields) && fields[i+1] == "default" {
				t.Errorf("exec argv keeps the placeholder namespace: %s", strings.TrimSpace(line))
			}
		}
	}
	if scanned == 0 {
		t.Fatal("no exec argv lines found in the applier block")
	}
}
