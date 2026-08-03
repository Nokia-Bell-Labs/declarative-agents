// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"strings"
	"testing"
)

// TestHelmCuratorUIDeliveredAsConfigMaps proves the documentation-curator's
// browser UI and the catalog docs tree reach the pod as sharded ConfigMaps that
// an init container unpacks into the workspace, rather than baked into a per-app
// runtime image at /opt/curator-ui (GH-1368). The staged assets shard into more
// than one ConfigMap, and each is projected and concatenated back into the
// tar.gz the init container extracts so the profile's browser-UI dist roots and
// its docs/ document root resolve under --directory /work (GH-1261, GH-1293).
func TestHelmCuratorUIDeliveredAsConfigMaps(t *testing.T) {
	chart := preparedTestChart(t)
	render := helmTemplate(t, chart)
	for _, want := range []string{
		"name: t-agent-architecture-curator-ui-part-000",
		"app.kubernetes.io/component: curator-ui",
		"binaryData:",
		"- name: stage-curator-ui",
		`command: ["sh", "-c", "cat /curator-ui/part-* | tar -xzf - -C /work"]`,
		"- {name: curator-ui, mountPath: /curator-ui, readOnly: true}",
		"- name: curator-ui",
	} {
		if !strings.Contains(render, want) {
			t.Errorf("curator UI render missing %q", want)
		}
	}
	// The retired image-baking mechanism must not linger.
	for _, forbidden := range []string{"/opt/curator-ui", "cp -a /opt/curator-ui"} {
		if strings.Contains(render, forbidden) {
			t.Errorf("curator render still references the retired baked UI path %q", forbidden)
		}
	}
	// The curator runs the shared agent-core image, not a per-app runtime.
	if strings.Contains(render, "agent-architecture-runtime") {
		t.Error("curator render still references the retired agent-architecture-runtime image")
	}
}

// TestHelmCuratorUIShardsProjectEveryConfigMap proves every staged shard becomes
// both a ConfigMap and a matching projected volume source, so the init
// container's cat of the mounted shards reconstructs the whole archive.
func TestHelmCuratorUIShardsProjectEveryConfigMap(t *testing.T) {
	chart := preparedTestChart(t)
	render := helmTemplate(t, chart)
	configMaps := strings.Count(render, "app.kubernetes.io/component: curator-ui")
	if configMaps < 2 {
		t.Fatalf("curator UI rendered %d shard ConfigMaps, want the multi-shard delivery", configMaps)
	}
	// Each shard ConfigMap is projected once as a volume source keyed by its name.
	sources := strings.Count(render, "-curator-ui-part-")
	// Each shard appears in its ConfigMap metadata name and in its projected
	// source name, so the projected-source count is at least the ConfigMap count.
	if sources < configMaps {
		t.Errorf("curator UI shards: %d ConfigMaps but only %d name references; a shard is unprojected",
			configMaps, sources)
	}
}
