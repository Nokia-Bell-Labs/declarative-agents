// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os/exec"
	"strings"
	"testing"
)

// applierExecBlock returns the agents__applier__exec-declarations.yaml value from
// a rendered profiles ConfigMap, up to the next ConfigMap key (machine.yaml sorts
// immediately after exec-declarations.yaml).
func applierExecBlock(rendered string) string {
	const marker = "agents__applier__exec-declarations.yaml:"
	i := strings.Index(rendered, marker)
	if i < 0 {
		return ""
	}
	rest := rendered[i:]
	if j := strings.Index(rest, "agents__applier__machine.yaml:"); j > 0 {
		return rest[:j]
	}
	return rest
}

// TestApplierExecDeclarationsRenderReleaseCoordinates proves the applier's
// helm/kubectl exec args target the installed release, namespace, and coding role
// Deployments rather than a baked coding-agent/default. It stages the chart
// through the production packaging path and renders under a non-default release
// name and namespace.
func TestApplierExecDeclarationsRenderReleaseCoordinates(t *testing.T) {
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

	// The release-derived coordinates must appear. The fullname for release relx
	// and chart coding-agent is relx-coding-agent.
	wantPresent := []string{
		"upgrade, relx,",
		"--namespace, nsy,",
		"rollback, relx]",
		"deployment/relx-coding-agent-planner,",
		"deployment/relx-coding-agent-executor,",
		"deployment/relx-coding-agent-critic,",
	}
	for _, w := range wantPresent {
		if !strings.Contains(block, w) {
			t.Errorf("applier exec args missing %q under release relx/nsy", w)
		}
	}
	// The baked defaults must be gone.
	wantAbsent := []string{
		"upgrade, coding-agent,",
		"namespace, default,",
		"rollback, coding-agent]",
		"deployment/coding-agent-planner,",
	}
	for _, w := range wantAbsent {
		if strings.Contains(block, w) {
			t.Errorf("applier exec args still carry baked default %q", w)
		}
	}
}
