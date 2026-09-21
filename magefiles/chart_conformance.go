// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/chartconf"
	"gopkg.in/yaml.v3"
)

// chartConformanceBaselinePath is the checked-in set of accepted violations,
// relative to the repository root (srd005 R7).
const chartConformanceBaselinePath = "docs/chart-conformance-baseline.yaml"

// conformanceApplications are the application charts the gate renders. The
// list is the same one TestApplicationKindRendersHaveTypeMeta walks; a new
// application joins both.
var conformanceApplications = []string{"agent-architecture", "chatbot-mesh", "coding-agent"}

// defaultsOverlay names the render that uses no values file, so a finding
// there reads the same way as one from a named overlay (srd005 R1.2).
const defaultsOverlay = "defaults"

// valuesOverlays returns the checked-in values files in a chart's ci/
// directory. A ci/ directory also holds cluster configurations and
// cluster-scoped fixtures, which are not values and would fail a render, so
// each file is classified by content rather than by name.
func valuesOverlays(chartSource string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(chartSource, "ci"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read chart ci directory: %w", err)
	}
	var overlays []string
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		path := filepath.Join(chartSource, "ci", entry.Name())
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		if isChartValues(string(content)) {
			overlays = append(overlays, entry.Name())
		}
	}
	sort.Strings(overlays)
	return overlays, nil
}

// isChartValues reports whether a document is a Helm values overlay rather
// than a Kubernetes object or a kind cluster configuration. A values file is
// one YAML mapping that names no apiVersion and kind pair.
func isChartValues(content string) bool {
	if chartconf.IsKindConfig(content) {
		return false
	}
	var document struct {
		APIVersion string `yaml:"apiVersion"`
		Kind       string `yaml:"kind"`
	}
	if err := yamlUnmarshalStrictDocument(content, &document); err != nil {
		return false
	}
	return document.APIVersion == "" || document.Kind == ""
}

// renderChart runs helm template over a staged chart, optionally with one
// values overlay, and returns the rendered manifests.
func renderChart(chart, overlay string) (string, error) {
	arguments := []string{"template", "conformance", chart}
	if overlay != defaultsOverlay {
		arguments = append(arguments, "-f", filepath.Join(chart, "ci", overlay))
	}
	out, err := exec.Command("helm", arguments...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("helm template %s [%s]: %w\n%s", chart, overlay, err, out)
	}
	return string(out), nil
}

// checkedInKindConfigs walks the repository for kind cluster configurations.
// Generated trees carry copies that are not the checked-in source, so they
// are skipped rather than reported twice.
func checkedInKindConfigs(root string) ([]string, error) {
	skipped := map[string]bool{
		".git": true, "node_modules": true, "build": true, "dist": true, "out": true,
	}
	var configs []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if skipped[entry.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(entry.Name()) != ".yaml" {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if chartconf.IsKindConfig(string(content)) {
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			configs = append(configs, filepath.ToSlash(relative))
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk for kind configurations: %w", err)
	}
	sort.Strings(configs)
	return configs, nil
}

// loadChartConformanceBaseline reads the checked-in baseline. An absent file
// means no violation is accepted, which is the state the release aims at.
func loadChartConformanceBaseline(root string) (chartconf.Baseline, error) {
	content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(chartConformanceBaselinePath)))
	if err != nil {
		if os.IsNotExist(err) {
			return chartconf.Baseline{}, nil
		}
		return chartconf.Baseline{}, fmt.Errorf("read %s: %w", chartConformanceBaselinePath, err)
	}
	return chartconf.LoadBaseline(string(content))
}

// helmMissingMessage names the rules a run could not check, so a skip says
// what went unchecked rather than only that it skipped.
func helmMissingMessage() string {
	return "helm not on PATH: rules " +
		strings.Join([]string{"R1", "R2", "R3", "R4", "R5", "R8"}, ", ") +
		" need a render and did not run; R6 does not"
}

// yamlUnmarshalStrictDocument decodes exactly one YAML document, so a
// multi-document file is not mistaken for a single mapping.
func yamlUnmarshalStrictDocument(content string, target any) error {
	decoder := yaml.NewDecoder(strings.NewReader(content))
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return fmt.Errorf("more than one YAML document")
	}
	return nil
}
