// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/apprig"
	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
	"gopkg.in/yaml.v3"
)

func TestApplicationRunnerTargetsSharedPlatformCoordinates(t *testing.T) {
	runner, err := codingApplicationRunner()
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := apprig.LoadManifest(runner.ManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := apprig.Resolve(manifest, runner.Binding)
	if err != nil {
		t.Fatal(err)
	}
	if runner.Binding.Cluster != kindrig.PlatformClusterName ||
		resolved.Namespace != codingApplicationNamespace ||
		resolved.Release != codingApplicationRelease ||
		resolved.BucketURL != "gs://coding-agent-telemetry" {
		t.Fatalf("shared application coordinates = %+v, binding=%+v", resolved, runner.Binding)
	}
}

func TestApplicationOverridesEnableDurableCollectorStorage(t *testing.T) {
	resolved := apprig.Resolved{
		Application: "coding-agent", Namespace: codingApplicationNamespace,
		BucketURL: "gs://coding-agent-telemetry", ObjectPrefix: "coding-agent",
	}
	images := codingHelmImages{Agent: "example/coding:revision"}
	var values map[string]any
	if err := yaml.Unmarshal([]byte(codingApplicationOverrides(resolved, images)), &values); err != nil {
		t.Fatal(err)
	}
	collector := values["collector"].(map[string]any)
	storage := collector["storage"].(map[string]any)
	if storage["backend"] != "object" ||
		storage["bucketName"] != "coding-agent-telemetry" ||
		storage["endpoint"] != kindrig.FakeGCSEndpoint ||
		storage["namespace"] != codingApplicationNamespace {
		t.Fatalf("collector storage overrides = %#v", storage)
	}
}

func TestApplicationIngressDeclaresRoleAndCollectorSurfaces(t *testing.T) {
	roots, err := resolveIntegrationRoots()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(roots.Application, "helm", "templates", "ingress.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	for _, surface := range []string{
		"hosts.planner", "hosts.plannerHealth", "hosts.executor", "hosts.critic", "hosts.collector",
		"port: {name: request}", "port: {name: control}", "port: {name: query}",
	} {
		if !strings.Contains(source, surface) {
			t.Errorf("shared ingress misses %q", surface)
		}
	}
}
