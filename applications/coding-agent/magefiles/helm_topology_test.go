// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestHelmPrepareStagesPackageAsOnlyProfileSource(t *testing.T) {
	packageRoot, _, cleanup := packageCanonicalDeployment(t)
	defer cleanup()
	chart := filepath.Join(t.TempDir(), "helm")
	if err := os.MkdirAll(chart, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := prepareHelmProfiles(packageRoot, chart); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{
		"deployment-manifest.yaml",
		"manifests/applier.yaml",
		"manifests/planner.yaml",
		"applier/applications/coding-agent/applier/profile.yaml",
		"planner/applications/coding-agent/planner/profile.yaml",
		"executor/applications/coding-agent/executor/profile.yaml",
		"critic/applications/coding-agent/critic/profile.yaml",
	} {
		if _, err := os.Stat(filepath.Join(chart, "profiles", filepath.FromSlash(rel))); err != nil {
			t.Errorf("prepared chart missing %s: %v", rel, err)
		}
	}
}

func TestHelmPrepareRejectsPackageWithoutManifest(t *testing.T) {
	err := prepareHelmProfiles(t.TempDir(), filepath.Join(t.TempDir(), "helm"))
	if err == nil || !strings.Contains(err.Error(), "no deployment manifest") {
		t.Fatalf("missing manifest error = %v", err)
	}
}

func TestHelmCoreTopologyRendersRoleContract(t *testing.T) {
	chart := preparedTestChart(t)
	render := helmTemplate(t, chart)
	for _, role := range servingRoles {
		for _, want := range []string{
			"name: test-coding-agent-" + role,
			`app.kubernetes.io/component: ` + role,
			`- "/profiles/applications/coding-agent/` + role + `/profile.yaml"`,
			"name: test-coding-agent-" + role + "-profiles-0",
		} {
			if !strings.Contains(render, want) {
				t.Errorf("%s render missing %q", role, want)
			}
		}
	}
	if got := strings.Count(render, "kind: Deployment"); got != 4 {
		t.Errorf("default Deployments = %d, want three roles plus collector agent", got)
	}
	// Every agent workload's main container runs the one application image
	// (srd005 R9, GH-2494): the three serving roles and the collector.
	if got := strings.Count(render,
		`image: "ghcr.io/nokia-bell-labs/declarative-agents/agent-core:0.1.0"`); got != 4 {
		t.Errorf("shared agent-core image count = %d, want 4 (3 roles + collector)", got)
	}
	// The executor receives Go and the linter from separate upstream donors.
	// The critic receives only Go through its own volume for its independent
	// changed-workspace oracle; no role runs an alternate agent image.
	for _, donor := range []string{"go-tool-donor", "golangci-lint-tool-donor"} {
		if got := strings.Count(render, "name: "+donor); got != 1 {
			t.Errorf("%s init containers = %d, want 1", donor, got)
		}
	}
	if got := strings.Count(render, "name: critic-go-tool-donor"); got != 1 {
		t.Errorf("critic Go donor init containers = %d, want 1", got)
	}
	goImage := `image: "docker.io/library/golang:1.26-alpine@sha256:8ac98ca534ac3f51e1f420a1dd2c15e74c75cfa0f23f3ad27eb5d7236c349a0c"`
	if got := strings.Count(render, goImage); got != 2 {
		t.Errorf("Go donor image count = %d, want executor and critic", got)
	}
	lintImage := `image: "docker.io/golangci/golangci-lint:v2.12.2-alpine@sha256:91b27804074a0bacea298707f016911e60cf0cdbc6c7bf5ccacb5f0606d18d60"`
	if got := strings.Count(render, lintImage); got != 1 {
		t.Errorf("linter donor image count = %d, want executor only", got)
	}
	// Both donors write the same emptyDir; the executor mounts it read-only.
	if got := strings.Count(render, "name: executor-tools"); got != 4 {
		t.Errorf("executor-tools references = %d, want 4 (two donor mounts, main mount, volume)", got)
	}
	if !strings.Contains(render,
		`value: "/opt/go-tools/go/bin:/opt/go-tools/bin:/usr/local/bin:/usr/bin:/bin"`) {
		t.Error("executor render missing donated-tool PATH")
	}
	if got := strings.Count(render, "name: critic-tools"); got != 3 {
		t.Errorf("critic-tools references = %d, want donor mount, main mount, volume", got)
	}
	if !strings.Contains(render,
		`value: "/opt/critic-tools/go/bin:/usr/local/bin:/usr/bin:/bin"`) {
		t.Error("critic render missing donated-Go PATH")
	}
	if got := strings.Count(render, "checksum/profiles:"); got != 4 {
		t.Errorf("profile rollout checksum count = %d, want 4 manifest deployments", got)
	}
	// Three roles and the collector, each with a readiness and a liveness probe;
	// the collector's pair arrives with the shared library workload (GH-2045).
	if got := strings.Count(render, "httpGet: {path: /api/lifecycle/health, port: control}"); got != 8 {
		t.Errorf("truthful lifecycle probes = %d, want readiness+liveness for 3 roles and the collector", got)
	}
	for _, want := range []string{
		`value: "http://test-coding-agent-executor:18210"`,
		`value: "http://test-coding-agent-critic:18220"`,
		`- "--otel-otlp-endpoint"`,
		`- "test-coding-agent-collector:4317"`,
		`claimName: test-coding-agent-workspace`,
		`readOnly: true`,
		`automountServiceAccountToken: false`,
		`readOnlyRootFilesystem: true`,
	} {
		if !strings.Contains(render, want) {
			t.Errorf("core render missing %q", want)
		}
	}
	if strings.Contains(render, "kind: StatefulSet") ||
		strings.Contains(render, "app.kubernetes.io/component: ollama") {
		t.Error("default external-LLM render unexpectedly contains Ollama workload")
	}
}

func TestHelmProfileChangeRollsOnlyAffectedRole(t *testing.T) {
	chart := preparedTestChart(t)
	before := roleChecksums(t, helmTemplate(t, chart))
	plannerFile := filepath.Join(chart, "profiles", "planner", "applications",
		"coding-agent", "planner", "request-machine.yaml")
	file, err := os.OpenFile(plannerFile, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("\n# checksum test\n"); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	after := roleChecksums(t, helmTemplate(t, chart))
	if before["planner"] == after["planner"] {
		t.Error("planner profile edit did not change planner checksum")
	}
	for _, role := range []string{"executor", "critic"} {
		if before[role] != after[role] {
			t.Errorf("planner profile edit changed %s checksum", role)
		}
	}
}

func TestHelmSmallAndOllamaValuesRender(t *testing.T) {
	chart := preparedTestChart(t)
	small := helmTemplate(t, chart, "-f", filepath.Join(chart, "ci", "small-values.yaml"))
	if !strings.Contains(small, "storage: 256Mi") ||
		!strings.Contains(small, `value: "http://host.docker.internal:11434"`) {
		t.Fatal("small-footprint values did not render storage and external model endpoint")
	}
	ollama := helmTemplate(t, chart, "--set", "ollama.enabled=true")
	for _, want := range []string{
		"kind: StatefulSet",
		"name: test-coding-agent-ollama",
		"name: test-coding-agent-ollama-preload",
		"name: wait-for-models",
		`value: "http://test-coding-agent-ollama:11434"`,
	} {
		if !strings.Contains(ollama, want) {
			t.Errorf("in-cluster Ollama render missing %q", want)
		}
	}
}

func preparedTestChart(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	app := filepath.Clean("..")
	chart := filepath.Join(t.TempDir(), "helm")
	if err := copyTree(filepath.Join(app, "helm"), chart); err != nil {
		t.Fatal(err)
	}
	packageRoot, _, cleanup := packageCanonicalDeployment(t)
	t.Cleanup(cleanup)
	if err := prepareHelmProfiles(packageRoot, chart); err != nil {
		t.Fatal(err)
	}
	return chart
}

func helmTemplate(t *testing.T, chart string, extra ...string) string {
	t.Helper()
	args := []string{"template", "test", chart}
	args = append(args, extra...)
	output, err := exec.Command("helm", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, output)
	}
	return string(output)
}

// TestHelmCollectorServesUI proves the collector serves the staged trace UI:
// ui/dist reaches the container through the profiles projection, and the
// container working directory is the profile directory so the literal ui/dist
// root resolves without an environment variable (srd020 R7, GH-1254).
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

func TestHelmCollectorProjectsRequestMachine(t *testing.T) {
	chart := preparedTestChart(t)
	render := helmTemplate(t, chart)
	for _, want := range []string{
		"agents__collector__query-machine.yaml",
		"path: agents/collector/query-machine.yaml",
	} {
		if !strings.Contains(render, want) {
			t.Errorf("collector request-machine render missing %q", want)
		}
	}
}

func TestHelmCollectorMountFollowsGeneratedManifest(t *testing.T) {
	chart := preparedTestChart(t)
	manifestPath := filepath.Join(chart, "profiles", "manifests", "collector.yaml")
	var manifest rolePackageManifest
	readYAMLFile(t, manifestPath, &manifest)
	manifest.Profile = "agents/collector-alternate/profile.yaml"
	if err := writeYAML(manifestPath, manifest); err != nil {
		t.Fatal(err)
	}

	render := helmTemplate(t, chart)
	for _, want := range []string{
		`workingDir: /profiles/agents/collector-alternate`,
		`- "/profiles/agents/collector-alternate/profile.yaml"`,
	} {
		if !strings.Contains(render, want) {
			t.Errorf("collector render did not follow generated manifest: missing %q", want)
		}
	}
}

func roleChecksums(t *testing.T, render string) map[string]string {
	t.Helper()
	pattern := regexp.MustCompile(`(?s)kind: Deployment\nmetadata:\n  name: test-coding-agent-(planner|executor|critic)\n.*?checksum/profiles: ([0-9a-f]{64})`)
	checksums := map[string]string{}
	for _, match := range pattern.FindAllStringSubmatch(render, -1) {
		if _, exists := checksums[match[1]]; !exists {
			checksums[match[1]] = match[2]
		}
	}
	if len(checksums) != 3 {
		t.Fatalf("rendered checksums = %#v, want all roles", checksums)
	}
	return checksums
}
