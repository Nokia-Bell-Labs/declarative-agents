// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// These helpers back the applier test surface (test-rel08.0-applier). They read
// the shipped applier profile and stage the coding chart with the applier
// enabled, mirroring the chatbot-mesh applier tests retargeted to coding-agent's
// serving-tree profile path and deployment names.

// agentDir returns the directory of a serving-tree agent profile. The applier is
// a serving-tree profile (agents/serving/applier), not a canonical role packaged
// through deployment.serving_profiles.
func agentDir(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join("..", "agents", "serving", name)
}

// readIntakeYAML decodes a profile YAML file into a partial struct. It is
// deliberately lenient -- unlike readStrictYAML -- because the applier test
// structs read only the fields under assertion.
func readIntakeYAML(t *testing.T, path string, target any) {
	t.Helper()
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if err := yaml.Unmarshal(data, target); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
}

// findChartDir returns the source Helm chart directory. The coding chart
// validates values against values.schema.json before templating, so a schema
// fixture is rejected against the source chart without staged profiles; only the
// applier render tests need the staged chart preparedApplierChart returns.
func findChartDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "helm"))
	if err != nil {
		t.Fatalf("resolve chart dir: %v", err)
	}
	return dir
}

// preparedApplierChart stages the chart with its serving profiles and the
// application-owned applier profile, so an applier-enabled render carries the
// applier ConfigMap the Deployment mounts. preparedTestChart alone stages the
// serving roles and the collector but not the applier, which is special-cased
// the same way (stageApplierProfile).
func preparedApplierChart(t *testing.T) string {
	t.Helper()
	chart := preparedTestChart(t)
	appRoot, err := resolveApplicationRoot("applier chart test")
	if err != nil {
		t.Fatal(err)
	}
	if err := stageApplierProfile(appRoot, chart); err != nil {
		t.Fatalf("stage applier profile: %v", err)
	}
	return chart
}
