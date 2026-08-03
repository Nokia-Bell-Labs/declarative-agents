// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"strings"
	"testing"
)

// TestHelmCuratorUIReferencesOutOfReleaseShards proves the documentation-curator
// mounts its browser UI and catalog docs from shard ConfigMaps provisioned
// OUTSIDE the Helm release (curatorUI.shards) rather than baked into a per-app
// runtime image at /opt/curator-ui (GH-1368) or carried in-release, which the
// gzipped UI would push past the 3 MiB release limit (GH-1402). When the shard
// names are supplied, the curator gains an init container that concatenates the
// mounted shards back into the tar.gz it unpacks into /work, and a projected
// volume with one configMap source per named shard.
func TestHelmCuratorUIReferencesOutOfReleaseShards(t *testing.T) {
	chart := preparedTestChart(t)
	render := helmTemplate(t, chart,
		"--set", "curatorUI.shards[0]=smoke-curator-ui-000",
		"--set", "curatorUI.shards[1]=smoke-curator-ui-001",
	)
	for _, want := range []string{
		"- name: stage-curator-ui",
		`command: ["sh", "-c", "cat /curator-ui/part-* | tar -xzf - -C /work"]`,
		"- {name: curator-ui, mountPath: /curator-ui, readOnly: true}",
		"- name: curator-ui",
		"name: smoke-curator-ui-000",
		"name: smoke-curator-ui-001",
	} {
		if !strings.Contains(render, want) {
			t.Errorf("curator UI render missing %q", want)
		}
	}
	// The chart must NOT emit the UI bytes itself: the shards are out-of-release,
	// so no curator-ui ConfigMap or binaryData block may appear in the release.
	for _, forbidden := range []string{
		"app.kubernetes.io/component: curator-ui",
		"binaryData:",
		"/opt/curator-ui",
		"agent-architecture-runtime",
	} {
		if strings.Contains(render, forbidden) {
			t.Errorf("curator render still carries the retired in-image/in-release UI marker %q", forbidden)
		}
	}
}

// TestHelmCuratorUIOmittedWithoutShards proves a bare render with no shard names
// omits the init container and the curator-ui volume entirely, so a plain
// install (or the applier-live tier, which does not exercise the UI) brings the
// curator up without it rather than failing on absent ConfigMaps.
func TestHelmCuratorUIOmittedWithoutShards(t *testing.T) {
	chart := preparedTestChart(t)
	render := helmTemplate(t, chart)
	for _, forbidden := range []string{
		"- name: stage-curator-ui",
		"cat /curator-ui/part-*",
		"- name: curator-ui",
	} {
		if strings.Contains(render, forbidden) {
			t.Errorf("curator render should omit the UI wiring without curatorUI.shards, but found %q", forbidden)
		}
	}
}
