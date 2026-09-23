// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// The harness rejects an empty coordinate, so every field this application
// owns is populated before the run starts. Kubeconfig and OverridesPath are
// the harness's to fill.
func TestDeployCoordinatesResolveCompletely(t *testing.T) {
	t.Parallel()
	coordinates := deployCoordinates(
		roots{Application: "/repo/applications/agent-architecture"},
		"/repo/applications/agent-architecture/build/deploy/demo/chart/agent-architecture-0.1.0.tgz")
	for name, value := range map[string]string{
		"release":   coordinates.Release,
		"namespace": coordinates.Namespace,
		"chart":     coordinates.ChartPath,
		"values":    coordinates.ValuesPath,
		"timeout":   coordinates.Timeout,
	} {
		if strings.TrimSpace(value) == "" {
			t.Errorf("coordinate %s is empty", name)
		}
	}
	if coordinates.Release != demoRelease {
		t.Errorf("release = %q, want %q", coordinates.Release, demoRelease)
	}
	if coordinates.Namespace != demoNamespace {
		t.Errorf("namespace = %q, want %q", coordinates.Namespace, demoNamespace)
	}
	if !strings.HasSuffix(coordinates.ValuesPath, "helm/ci/kind-values.yaml") {
		t.Errorf("values = %q, want the checked-in kind overlay", coordinates.ValuesPath)
	}
}

type deployOverridesDocument struct {
	Image struct {
		Repository any `yaml:"repository"`
		Tag        any `yaml:"tag"`
	} `yaml:"image"`
	Collector struct {
		Image struct {
			Repository any `yaml:"repository"`
			Tag        any `yaml:"tag"`
		} `yaml:"image"`
	} `yaml:"collector"`
	CuratorUI struct {
		Shards []string `yaml:"shards"`
	} `yaml:"curatorUI"`
}

func decodeOverrides(t *testing.T, document string) deployOverridesDocument {
	t.Helper()
	var decoded deployOverridesDocument
	if err := yaml.Unmarshal([]byte(document), &decoded); err != nil {
		t.Fatalf("overrides do not decode as YAML: %v\n%s", err, document)
	}
	return decoded
}

// A commit-shaped tag decodes as an integer when bare and the image reference
// stops resolving. --set-string prevented that before the migration, so the
// document has to keep the quotes. Assert the type, not the value.
func TestDeployOverridesKeepImageTagsAsStrings(t *testing.T) {
	t.Parallel()
	decoded := decodeOverrides(t, deployOverrides("agent-core:20260919", []string{"shard-a"}))
	for name, value := range map[string]any{
		"image.tag":           decoded.Image.Tag,
		"collector.image.tag": decoded.Collector.Image.Tag,
	} {
		if _, ok := value.(string); !ok {
			t.Errorf("%s decoded as %T (%v), want a string", name, value, value)
		}
	}
	if decoded.Image.Tag != "20260919" {
		t.Errorf("image.tag = %v, want the numeric-looking tag preserved", decoded.Image.Tag)
	}
}

// Both workloads run the one locally built agent-core image (GH-1368), which
// is why the same reference appears twice.
func TestDeployOverridesGiveBothWorkloadsTheSameImage(t *testing.T) {
	t.Parallel()
	decoded := decodeOverrides(t, deployOverrides("registry.local/agent-core:abc123", nil))
	if decoded.Image.Repository != "registry.local/agent-core" {
		t.Errorf("image.repository = %v", decoded.Image.Repository)
	}
	if decoded.Collector.Image.Repository != decoded.Image.Repository {
		t.Errorf("collector repository = %v, want the same image as the workload",
			decoded.Collector.Image.Repository)
	}
	if decoded.Collector.Image.Tag != decoded.Image.Tag {
		t.Errorf("collector tag = %v, want the same tag as the workload", decoded.Collector.Image.Tag)
	}
}

// curatorUI.shards was a repeated --set curatorUI.shards[N]=name. The same
// list has to arrive as a list, in the order it was provisioned.
func TestDeployOverridesCarryTheShardsInOrder(t *testing.T) {
	t.Parallel()
	shards := []string{"demo-curator-ui-0", "demo-curator-ui-1", "demo-curator-ui-2"}
	decoded := decodeOverrides(t, deployOverrides(
		"localhost/declarative-agents/runtime/agent-core:git-0123456789ab-linux-arm64", shards))
	if len(decoded.CuratorUI.Shards) != len(shards) {
		t.Fatalf("shards = %v, want %v", decoded.CuratorUI.Shards, shards)
	}
	for index, want := range shards {
		if decoded.CuratorUI.Shards[index] != want {
			t.Errorf("shard %d = %q, want %q", index, decoded.CuratorUI.Shards[index], want)
		}
	}
}

// An application with no shards still produces a document that decodes, and
// an empty list rather than a null the chart would have to guard.
func TestDeployOverridesWriteAnEmptyShardListAsAList(t *testing.T) {
	t.Parallel()
	document := deployOverrides(
		"localhost/declarative-agents/runtime/agent-core:git-0123456789ab-linux-arm64", nil)
	decoded := decodeOverrides(t, document)
	if decoded.CuratorUI.Shards == nil {
		t.Errorf("empty shards decoded as null; document:\n%s", document)
	}
	if len(decoded.CuratorUI.Shards) != 0 {
		t.Errorf("shards = %v, want empty", decoded.CuratorUI.Shards)
	}
}

// The verbs bind the catalog's own profile variants rather than a copy, so a
// machine change in the catalog reaches this application.
func TestDeployProfilesAreCatalogRelative(t *testing.T) {
	t.Parallel()
	for name, rel := range map[string]string{
		"deploy":   applierDeployProfileRel,
		"undeploy": applierUndeployProfileRel,
	} {
		if !strings.HasPrefix(rel, "agents/applier/") {
			t.Errorf("%s profile %q is not catalog-relative", name, rel)
		}
		if !strings.HasSuffix(rel, "-profile.yaml") {
			t.Errorf("%s profile %q does not name a profile variant", name, rel)
		}
	}
}
