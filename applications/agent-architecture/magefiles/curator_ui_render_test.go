// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

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

// TestHelmCuratorResolvesItsStaticAssetRoots proves the curator container runs
// with its working directory at the workspace the UI shards unpack into.
//
// The curator's static_assets roots are literal, profile-relative paths, and
// serveStaticAssets opens them under http.Dir(root) — resolved against the
// process working directory, never joined to the profile or the agent
// directory. The agent-core image declares WorkingDir /, so without an
// explicit workingDir the roots addressed /agents/... while the shards unpack
// to /work/agents/..., and the documentation and monitor UIs answered 404
// while the API answered 200 (GH-2348).
//
// The working directory has to match the init container's unpack target, so
// this asserts the pair rather than the literal alone: a chart that moved the
// unpack and not the working directory would reintroduce the same 404.
func TestHelmCuratorResolvesItsStaticAssetRoots(t *testing.T) {
	chart := preparedTestChart(t)
	render := helmTemplate(t, chart, "--set", "curatorUI.shards[0]=smoke-curator-ui-000")
	if !strings.Contains(render, "workingDir: /work") {
		t.Error("curator container declares no workingDir; its static_assets roots resolve against the process working directory, which the image sets to /")
	}
	if !strings.Contains(render, "tar -xzf - -C /work") {
		t.Error("the UI shards no longer unpack into /work; the working directory above has to follow them")
	}
	if !strings.Contains(render, "- {name: workspace, mountPath: /work}") {
		t.Error("the curator no longer mounts the workspace at /work")
	}
}

// A render without shards still sets the working directory. The UI is what
// needs it, but a container whose working directory depends on whether an
// optional value was supplied is a container that behaves differently in the
// tier that does not exercise the UI, which is where this defect hid.
func TestHelmCuratorWorkingDirectoryDoesNotDependOnTheShards(t *testing.T) {
	chart := preparedTestChart(t)
	if render := helmTemplate(t, chart); !strings.Contains(render, "workingDir: /work") {
		t.Error("curator working directory is conditional on curatorUI.shards")
	}
}
