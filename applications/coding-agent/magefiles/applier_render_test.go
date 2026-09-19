// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
)

// applierExecBlock returns the normalized applier exec-declarations value from
// a rendered profiles ConfigMap, up to the next ConfigMap key.
func applierExecBlock(rendered string) string {
	const marker = "applications__coding-agent__applier__exec-declarations.yaml:"
	i := strings.Index(rendered, marker)
	if i < 0 {
		return ""
	}
	rest := rendered[i:]
	if j := strings.Index(rest, "applications__coding-agent__applier__machine.yaml:"); j > 0 {
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

// assertCLIDonorRendered checks the applier Deployment carries the GH-2222 donor
// shape: a cli-donor init container on the pinned image, a tools emptyDir the
// applier mounts read-only at /opt/tools, and /opt/tools first on PATH.
func assertCLIDonorRendered(t *testing.T, rendered, donorImage string) {
	t.Helper()
	for _, want := range []string{
		"- name: cli-donor",
		"image: " + strconv.Quote(donorImage),
		`command: ["sh", "-c", "cp -f /usr/bin/helm /usr/bin/kubectl /tools/"]`,
		"- {name: tools, mountPath: /tools}",
		"- {name: tools, mountPath: /opt/tools, readOnly: true}",
		`- {name: PATH, value: "/opt/tools:/usr/local/bin:/usr/bin:/bin"}`,
		"- name: tools\n          emptyDir: {}",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("rendered applier lacks %q", want)
		}
	}
}

// TestApplierRendersPinnedCLIDonor proves the chart delivers helm and kubectl
// through the donor pinned to kindrig.CLIDonorImage, and that the kind overlay
// switches to the rig-local kind-loaded tag (GH-2222).
func TestApplierRendersPinnedCLIDonor(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	chart := preparedApplierChart(t)
	out, err := exec.Command("helm", "template", "relx", chart, "--set", "applier.enabled=true").CombinedOutput()
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	assertCLIDonorRendered(t, string(out), kindrig.CLIDonorImage)
	kind, err := exec.Command("helm", "template", "relx", chart, "--set", "applier.enabled=true",
		"-f", filepath.Join("..", "helm", "ci", "kind-applier-values.yaml")).CombinedOutput()
	if err != nil {
		t.Fatalf("helm template kind overlay: %v\n%s", err, kind)
	}
	assertCLIDonorRendered(t, string(kind), kindrig.CLIDonorRuntimeImage)
}
