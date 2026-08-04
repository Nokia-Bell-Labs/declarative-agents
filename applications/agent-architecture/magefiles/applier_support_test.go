// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// These helpers back the applier test surface (test-rel02.0 applier). They read
// the shipped applier profile and stage the agent-architecture chart with the
// applier enabled, mirroring the coding-agent applier tests retargeted to the
// agent-architecture profile path (agents/applier), chart, and Deployment names.

// agentDir returns the directory of an application-owned agent profile. The
// applier lives directly under agents/applier (it is application-owned and staged
// alongside the catalog closures, not a catalog-referenced role).
func agentDir(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join("..", "agents", name)
}

// readIntakeYAML decodes a profile YAML file into a partial struct. It is
// deliberately lenient because the applier test structs read only the fields under
// assertion.
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

// findChartDir returns the source Helm chart directory.
func findChartDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "helm"))
	if err != nil {
		t.Fatalf("resolve chart dir: %v", err)
	}
	return dir
}

// prepareChart stages the source chart with every manifest-declared deployment
// closure. Staging matters: the source chart cannot render without the generated
// packages and provenance.
func prepareChart(t *testing.T) string {
	t.Helper()
	resolved, err := resolveRootsFromWorkingDirectory()
	if err != nil {
		t.Fatalf("resolve roots: %v", err)
	}
	stage := t.TempDir()
	chart := filepath.Join(stage, "agent-architecture")
	if err := stageChartSource(filepath.Join(resolved.Application, "helm"), chart); err != nil {
		t.Fatalf("stage source chart: %v", err)
	}
	if err := prepareChartProfiles(resolved.Application, resolved.Catalog, chart); err != nil {
		t.Fatalf("stage manifest profiles: %v", err)
	}
	return chart
}

// preparedTestChart stages the chart for a default render. The applier package is
// present because the manifest declares it, while values keep its workload off.
func preparedTestChart(t *testing.T) string {
	t.Helper()
	return prepareChart(t)
}

// preparedApplierChart returns the same complete package for an applier-enabled
// render.
func preparedApplierChart(t *testing.T) string {
	t.Helper()
	return prepareChart(t)
}

// helmTemplate renders a prepared chart, skipping when helm is not on PATH.
func helmTemplate(t *testing.T, chart string, args ...string) string {
	t.Helper()
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	full := append([]string{"template", "t", chart}, args...)
	out, err := exec.Command("helm", full...).CombinedOutput()
	if err != nil {
		t.Fatalf("helm %v: %v\n%s", full, err, out)
	}
	return string(out)
}

// containsString reports whether values contains want.
func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// argAfter returns the value following flag in an argument list.
func argAfter(args []string, flag string) string {
	for i, arg := range args {
		if arg == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}
