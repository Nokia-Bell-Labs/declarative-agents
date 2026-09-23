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
	runner, err := architectureApplicationRunner()
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
		resolved.Namespace != architectureApplicationNamespace ||
		resolved.Release != architectureApplicationRelease ||
		resolved.BucketURL != "gs://agent-architecture-telemetry" {
		t.Fatalf("shared application coordinates = %+v, binding=%+v", resolved, runner.Binding)
	}
}

func TestApplicationOverridesEnableDurableCollectorStorage(t *testing.T) {
	resolved := apprig.Resolved{
		Application: "agent-architecture", Namespace: architectureApplicationNamespace,
		BucketURL: "gs://agent-architecture-telemetry", ObjectPrefix: "agent-architecture",
	}
	var values map[string]any
	if err := yaml.Unmarshal([]byte(architectureApplicationOverrides(
		resolved, "registry.local/agent-core:test", []string{"agent-architecture-curator-ui-000"})), &values); err != nil {
		t.Fatal(err)
	}
	collector := values["collector"].(map[string]any)
	storage := collector["storage"].(map[string]any)
	if storage["backend"] != "object" ||
		storage["bucketName"] != "agent-architecture-telemetry" ||
		storage["endpoint"] != kindrig.FakeGCSEndpoint ||
		storage["namespace"] != architectureApplicationNamespace {
		t.Fatalf("collector storage overrides = %#v", storage)
	}
}

func TestApplicationIngressDeclaresEveryBrowserSurface(t *testing.T) {
	root, err := resolveRootsFromWorkingDirectory()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root.Application, "helm", "templates", "ingress.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	for _, surface := range []string{
		"hosts.documentation", "hosts.lifecycle", "hosts.monitor", "hosts.collector",
		"port: {name: documentation}", "port: {name: control}",
		"port: {name: monitor}", "port: {name: query}",
	} {
		if !strings.Contains(source, surface) {
			t.Errorf("shared ingress misses %q", surface)
		}
	}
}
