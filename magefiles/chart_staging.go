// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/helmlib"
)

// stagedProfileClosures are the fixture files a chart needs before it will
// render outside its application's own mage run. Each chart fails closed
// without the closure mage helmPrepare stages, so a render from here stages
// an equivalent rather than reaching into another module's targets.
var stagedProfileClosures = map[string]map[string]string{
	"agent-architecture": {
		"profiles/prepared-manifest.yaml": "mount_path: /profiles\nroles:\n" +
			"  - role: curator\n    profile: profile.yaml\n" +
			"  - role: collector\n    profile: profile.yaml\n" +
			"  - role: applier\n    profile: profile.yaml\n",
		"profiles/curator/profile.yaml":   "fixture: true\n",
		"profiles/collector/profile.yaml": "fixture: true\n",
	},
	"coding-agent": {
		"profiles/manifests/planner.yaml":   codingRoleManifest,
		"profiles/manifests/executor.yaml":  codingRoleManifest,
		"profiles/manifests/critic.yaml":    codingRoleManifest,
		"profiles/manifests/collector.yaml": codingRoleManifest,
		"profiles/planner/profile.yaml":     "fixture: true\n",
		"profiles/executor/profile.yaml":    "fixture: true\n",
		"profiles/critic/profile.yaml":      "fixture: true\n",
		"profiles/collector/profile.yaml":   "fixture: true\n",
	},
}

const codingRoleManifest = "profile: profile.yaml\nfiles:\n  - profile.yaml\n" +
	"config_maps:\n  - index: 0\n    files:\n      - profile.yaml\n"

// stageChartForRender copies an application chart to a temporary directory,
// vendors the shared library chart Helm resolves only from the chart's own
// charts/ directory (GH-2045), and writes the staged profile closures the
// templates require. It returns the staged chart and a cleanup to call when
// the render is done.
//
// The applier closure is staged for every chart because a values overlay can
// turn the applier on and the templates fail closed without it (GH-2408).
func stageChartForRender(root, application string) (string, func(), error) {
	source := filepath.Join(root, "applications", application, "helm")
	staging, err := os.MkdirTemp("", "chart-render-"+application+"-*")
	if err != nil {
		return "", func() {}, fmt.Errorf("stage chart: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(staging) }
	chart := filepath.Join(staging, application)
	if err := os.CopyFS(chart, os.DirFS(source)); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("copy chart %s: %w", application, err)
	}
	if err := helmlib.Vendor(root, chart); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("vendor library chart: %w", err)
	}
	closures := map[string]string{"profiles/applier/profile.yaml": "fixture: true\n"}
	for path, content := range stagedProfileClosures[application] {
		closures[path] = content
	}
	for path, content := range closures {
		target := filepath.Join(chart, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			cleanup()
			return "", func() {}, err
		}
		if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
			cleanup()
			return "", func() {}, err
		}
	}
	return chart, cleanup, nil
}
