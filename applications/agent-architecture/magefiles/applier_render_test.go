// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os/exec"
	"strings"
	"testing"
)

// applierExecBlock returns the local wrapper's exec-declarations value from a
// rendered profiles ConfigMap, up to the next ConfigMap key (machine.yaml sorts
// immediately after exec-declarations.yaml).
func applierExecBlock(rendered string) string {
	const marker = "applications__agent-architecture__applier__exec-declarations.yaml:"
	i := strings.Index(rendered, marker)
	if i < 0 {
		return ""
	}
	rest := rendered[i:]
	if j := strings.Index(rest, "applications__agent-architecture__applier__profile.yaml:"); j > 0 {
		return rest[:j]
	}
	return rest
}

// TestApplierExecDeclarationsRenderReleaseCoordinates proves the applier's
// helm/kubectl exec args target the installed release, namespace, and collector
// Deployment rather than a baked agent-architecture/default. It stages the chart
// through the production packaging path and renders under a non-default release name
// and namespace.
func TestApplierExecDeclarationsRenderReleaseCoordinates(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	chart := preparedApplierChart(t)
	out, err := exec.Command("helm", "template", "relx", chart,
		"--namespace", "nsy", "--set", "applier.enabled=true").CombinedOutput()
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	block := applierExecBlock(string(out))
	if block == "" {
		t.Fatal("applier exec-declarations key not found in rendered ConfigMap")
	}

	// The fullname for release relx and chart agent-architecture is
	// relx-agent-architecture.
	wantPresent := []string{
		"upgrade, relx,",
		"--namespace, nsy,",
		"rollback, relx]",
		"deployment/relx-agent-architecture-collector,",
	}
	for _, w := range wantPresent {
		if !strings.Contains(block, w) {
			t.Errorf("applier exec args missing %q under release relx/nsy", w)
		}
	}
	wantAbsent := []string{
		"upgrade, agent-architecture,",
		"namespace, default,",
		"rollback, agent-architecture]",
		"deployment/agent-architecture-collector,",
	}
	for _, w := range wantAbsent {
		if strings.Contains(block, w) {
			t.Errorf("applier exec args still carry baked default %q", w)
		}
	}
}
