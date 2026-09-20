// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// The harness rejects an empty coordinate, so every field this application
// owns has to be populated before the run starts. Kubeconfig and OverridesPath
// are the harness's to fill.
func TestCodingDeployCoordinatesResolveCompletely(t *testing.T) {
	t.Parallel()
	coordinates := codingDeployCoordinates(
		integrationRoots{Application: "/repo/applications/coding-agent"},
		"/repo/applications/coding-agent/build/deploy/demo/chart/coding-agent-1.0.0.tgz")
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
	if coordinates.Release != codingDemoRelease {
		t.Errorf("release = %q, want %q", coordinates.Release, codingDemoRelease)
	}
	if !strings.HasSuffix(coordinates.ValuesPath, "helm/ci/kind-values.yaml") {
		t.Errorf("values = %q, want the checked-in kind overlay", coordinates.ValuesPath)
	}
}

// A tag is a commit-shaped string that YAML reads as a number when it is bare:
// an unquoted 20260919 decodes as an integer and the image reference stops
// resolving. The imperative path this replaced used --set-string for the same
// reason, and losing the quoting is the one silent way this migration breaks.
func TestCodingDeployOverridesKeepImageTagsAsStrings(t *testing.T) {
	t.Parallel()
	overrides := codingDeployOverrides(codingHelmImages{Agent: "coding-agent:20260919"})

	var decoded struct {
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
	}
	if err := yaml.Unmarshal([]byte(overrides), &decoded); err != nil {
		t.Fatalf("overrides do not decode as YAML: %v\n%s", err, overrides)
	}
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
	if repository, _ := decoded.Image.Repository.(string); repository != "coding-agent" {
		t.Errorf("image.repository = %v, want coding-agent", decoded.Image.Repository)
	}
}

// The deploy and undeploy verbs bind the catalog's own profile variants rather
// than a copy, so a machine change in the catalog reaches this application.
func TestCodingDeployProfilesAreCatalogRelative(t *testing.T) {
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
