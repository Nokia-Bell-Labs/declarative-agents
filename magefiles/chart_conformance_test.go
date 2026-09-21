// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/chartconf"
)

// TestChartConformance is the gate the repository audit runs. It renders every
// application chart under its own defaults and under each checked-in values
// overlay, checks every rendered manifest and every checked-in kind
// configuration against the ENG01 chart rules, and reconciles what it finds
// against the checked-in exception baseline (srd005 R1 through R8).
//
// Helm is required to render. ENG01 lists it as required toolchain and the
// release gate has it, but a laptop without it still runs the rest of the
// audit, so a missing helm skips the rendered-manifest rules and names them
// rather than passing silently. R6 reads files and runs either way.
func TestChartConformance(t *testing.T) {
	root, err := findRepositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := loadChartConformanceBaseline(root)
	if err != nil {
		t.Fatal(err)
	}

	var findings []chartconf.Finding
	if _, err := exec.LookPath("helm"); err != nil {
		t.Log(helmMissingMessage())
	} else {
		findings = append(findings, renderedFindings(t, root)...)
	}
	findings = append(findings, kindConfigFindings(t, root)...)

	unaccepted, stale := chartconf.Reconcile(findings, baseline)
	if err := chartconf.Report(unaccepted, stale); err != nil {
		t.Fatal(err)
	}
	t.Logf("chart conformance: %d finding(s), all accepted by %s",
		len(findings), chartConformanceBaselinePath)
}

func renderedFindings(t *testing.T, root string) []chartconf.Finding {
	t.Helper()
	var findings []chartconf.Finding
	for _, application := range conformanceApplications {
		source := filepath.Join(root, "applications", application, "helm")
		overlays, err := valuesOverlays(source)
		if err != nil {
			t.Fatal(err)
		}
		chart := stageKindRenderChart(t, root, application)
		for _, overlay := range append([]string{defaultsOverlay}, overlays...) {
			rendered, err := renderChart(chart, overlay)
			if err != nil {
				t.Fatalf("render %s [%s]: %v", application, overlay, err)
			}
			documents, err := chartconf.Parse(rendered)
			if err != nil {
				t.Fatalf("parse %s [%s]: %v", application, overlay, err)
			}
			findings = append(findings, chartconf.Check(application, overlay, documents)...)
		}
	}
	return findings
}

func kindConfigFindings(t *testing.T, root string) []chartconf.Finding {
	t.Helper()
	configs, err := checkedInKindConfigs(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(configs) == 0 {
		t.Fatal("no checked-in kind configuration found; ENG01 requires at least the platform one")
	}
	var findings []chartconf.Finding
	for _, config := range configs {
		content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(config)))
		if err != nil {
			t.Fatal(err)
		}
		configFindings, err := chartconf.CheckKindConfig(config, string(content))
		if err != nil {
			t.Fatal(err)
		}
		findings = append(findings, configFindings...)
	}
	return findings
}

// A ci/ directory holds values overlays beside cluster configurations and
// cluster-scoped fixtures. Rendering a chart with a cluster configuration as
// its values file does not fail loudly, it renders something meaningless, so
// the classification is checked against the real directories.
func TestValuesOverlayClassification(t *testing.T) {
	root, err := findRepositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	expected := map[string][]string{
		"agent-architecture": {"kind-applier-values.yaml", "kind-values.yaml"},
		"chatbot-mesh":       {"kind-applier-values.yaml", "kind-llm-values.yaml", "kind-values.yaml"},
		"coding-agent":       {"kind-applier-values.yaml", "kind-values.yaml", "small-values.yaml"},
	}
	for application, want := range expected {
		overlays, err := valuesOverlays(filepath.Join(root, "applications", application, "helm"))
		if err != nil {
			t.Fatal(err)
		}
		if len(overlays) != len(want) {
			t.Errorf("%s: overlays %v, want %v", application, overlays, want)
			continue
		}
		for i := range want {
			if overlays[i] != want[i] {
				t.Errorf("%s: overlays %v, want %v", application, overlays, want)
				break
			}
		}
	}
}

func TestIsChartValuesRejectsNonValuesDocuments(t *testing.T) {
	cases := map[string]struct {
		content string
		want    bool
	}{
		"values overlay": {"image:\n  repository: agent-core\n  tag: \"0.1.0\"\n", true},
		"kind cluster configuration": {
			"apiVersion: kind.x-k8s.io/v1alpha4\nkind: Cluster\nnodes:\n  - role: control-plane\n", false},
		"kubernetes object": {
			"apiVersion: v1\nkind: PersistentVolume\nmetadata:\n  name: workspace\n", false},
		"multi-document fixture": {
			"apiVersion: v1\nkind: PersistentVolume\nmetadata:\n  name: a\n---\n" +
				"apiVersion: v1\nkind: PersistentVolumeClaim\nmetadata:\n  name: b\n", false},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			if got := isChartValues(testCase.content); got != testCase.want {
				t.Errorf("isChartValues = %v, want %v", got, testCase.want)
			}
		})
	}
}

// Every kind configuration ENG01 Table 2 names must be found by the walk, so
// a renamed or moved configuration cannot drop out of the gate silently.
func TestCheckedInKindConfigsFindsTheDocumentedOnes(t *testing.T) {
	root, err := findRepositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	configs, err := checkedInKindConfigs(root)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, config := range configs {
		found[config] = true
	}
	for _, want := range []string{
		"magefiles/kindrig/platform-kind-config.yaml",
		"applications/chatbot-mesh/testdata/kind-policy-config.yaml",
		"applications/agent-architecture/helm/ci/kind-demo-config.yaml",
		"applications/chatbot-mesh/helm/ci/kind-demo-config.yaml",
		"applications/coding-agent/helm/ci/kind-demo-config.yaml",
	} {
		if !found[want] {
			t.Errorf("%s not found; walk returned %v", want, configs)
		}
	}
	for _, unwanted := range []string{
		"magefiles/kindrig/traefik-kind.yaml",
		"magefiles/kindrig/metrics-server-kind.yaml",
	} {
		if found[unwanted] {
			t.Errorf("%s is a manifest, not a cluster configuration", unwanted)
		}
	}
}

// The baseline is read, never written, and every entry it carries is
// complete. A malformed baseline must fail here rather than silently
// suspending nothing.
func TestCheckedInBaselineLoads(t *testing.T) {
	root, err := findRepositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, filepath.FromSlash(chartConformanceBaselinePath))
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := loadChartConformanceBaseline(root); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("loading the baseline rewrote it")
	}
}
