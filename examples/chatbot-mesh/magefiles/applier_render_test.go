// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// applierExecBlock returns the agents__applier__exec-declarations.yaml value
// from a rendered profiles ConfigMap, up to the next ConfigMap key.
func applierExecBlock(rendered string) string {
	const marker = "agents__applier__exec-declarations.yaml:"
	i := strings.Index(rendered, marker)
	if i < 0 {
		return ""
	}
	rest := rendered[i:]
	if j := strings.Index(rest, "agents__applier__profile.yaml:"); j > 0 {
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
		"deployment/chatbot-mesh-chatbot,",
	}
	for _, w := range wantAbsent {
		if strings.Contains(block, w) {
			t.Errorf("applier exec args still carry baked default %q", w)
		}
	}
}
