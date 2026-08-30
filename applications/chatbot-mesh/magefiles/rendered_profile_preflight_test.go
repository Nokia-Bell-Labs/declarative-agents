// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// renderedConfigMap is the profiles ConfigMap as helm renders it: one data key
// per staged profile file, the path separators written as double underscores.
type renderedConfigMap struct {
	Kind     string `yaml:"kind"`
	Metadata struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Data map[string]string `yaml:"data"`
}

// renderChartProfiles renders the staged chart with the given --set overrides,
// writes the profiles ConfigMap back out as a directory tree, and returns its
// root. That tree is what a pod mounts, which is the thing this file exists to
// check: the source tree under agents/ and the rendered ConfigMap are different
// artifacts, and only the rendered one reflects co-generation.
func renderChartProfiles(t *testing.T, staged string, sets ...string) string {
	t.Helper()
	args := []string{"template", "rel", staged, "--namespace", "nsy"}
	for _, set := range sets {
		args = append(args, "--set", set)
	}
	out, err := exec.Command("helm", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("helm %s: %v\n%s", strings.Join(args, " "), err, out)
	}

	root := t.TempDir()
	var wrote int
	decoder := yaml.NewDecoder(strings.NewReader(string(out)))
	for {
		var doc renderedConfigMap
		if err := decoder.Decode(&doc); err != nil {
			break
		}
		if doc.Kind != "ConfigMap" || !strings.HasSuffix(doc.Metadata.Name, "-profiles") {
			continue
		}
		for key, body := range doc.Data {
			path := filepath.Join(root, filepath.Join(strings.Split(key, "__")...))
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatalf("create %s: %v", filepath.Dir(path), err)
			}
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				t.Fatalf("write %s: %v", path, err)
			}
			wrote++
		}
	}
	if wrote == 0 {
		t.Fatal("the rendered chart carries no profiles ConfigMap data; the check would pass vacuously")
	}
	return root
}

// preflightRenderedProfiles builds the agent and runs the same --validate-config
// load every profile takes at pod start, over the rendered tree.
func preflightRenderedProfiles(t *testing.T, root string) error {
	t.Helper()
	chartDir := findChartDir(t)
	applicationRoot := filepath.Dir(chartDir)
	coreRoot := demoCoreRoot(applicationRoot)
	if !agentCoreAvailable(coreRoot) {
		t.Skipf("agent-core checkout not found at %s", coreRoot)
	}
	binary, err := buildAgent(coreRoot)
	if err != nil {
		t.Fatalf("build agent: %v", err)
	}
	profiles, err := meshProfiles(root)
	if err != nil {
		t.Fatalf("collect rendered profiles: %v", err)
	}
	return bootSmokeProfiles(defaultSmokeRun, binary, coreRoot, profiles)
}

// TestRenderedProfilesPreflight closes the gap that lets a chart whose own
// default cannot start stay green through every gate.
//
// The mesh boot smoke in `mage audit` preflights the profiles under agents/,
// where rest.yaml declares every client unconditionally. A pod mounts the
// profiles ConfigMap instead, and some of the chatbot's keys are co-generated
// from values -- so a word the machine dispatches can lose the client it binds
// without anything under agents/ changing. That is not hypothetical: in the
// cohere-demo deployment of this application, the chart's default rendered a
// chatbot that refused to load with a "REST client is not defined" authority
// error while audit, the boot smoke, and the whole test suite stayed green
// (cohere-demo GH-223).
func TestRenderedProfilesPreflight(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	chartDir := findChartDir(t)
	staged, cleanup, err := stageSmokeChart(chartDir, filepath.Dir(chartDir))
	if err != nil {
		t.Fatalf("stage chart: %v", err)
	}
	defer cleanup()

	root := renderChartProfiles(t, staged)
	if err := preflightRenderedProfiles(t, root); err != nil {
		t.Errorf("the rendered chart cannot start at its own defaults: %v", err)
	}
}
