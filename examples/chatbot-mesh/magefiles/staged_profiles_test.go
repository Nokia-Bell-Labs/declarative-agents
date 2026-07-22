// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"testing"
)

var profileMountRE = regexp.MustCompile(`agents/([a-z0-9-]+)/profile\.yaml`)

// TestStagedProfilesCoverEnabledDeployments proves the authoritative packaging
// list (chartProfilePrograms) stages an agent profile for every profile mounted
// by a chart Deployment. This is the GH-485 regression guard: the executor
// Deployment mounts agents/executor/profile.yaml, so the staging list must copy
// agents/executor or an enabled executor starts with no profile.
func TestStagedProfilesCoverEnabledDeployments(t *testing.T) {
	chartDir := findChartDir(t)
	templatesDir := filepath.Join(chartDir, "templates")

	entries, err := os.ReadDir(templatesDir)
	if err != nil {
		t.Fatalf("read templates dir: %v", err)
	}

	mounted := map[string]bool{}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(templatesDir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		for _, m := range profileMountRE.FindAllStringSubmatch(string(data), -1) {
			mounted[m[1]] = true
		}
	}
	if len(mounted) == 0 {
		t.Fatal("no agent profile mounts found in chart templates; regex or layout changed")
	}

	staged := map[string]bool{}
	for _, p := range chartProfilePrograms() {
		staged[filepath.Base(p.src)] = true
	}

	var missing []string
	for agent := range mounted {
		if !staged[agent] {
			missing = append(missing, agent)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("chart Deployments mount profiles not staged by chartProfilePrograms: %v", missing)
	}
}

// TestPackagingDocMatchesStagingList proves PACKAGING.md documents exactly the
// programs the code stages, so a packaging change cannot be documentation-only
// (GH-485).
func TestPackagingDocMatchesStagingList(t *testing.T) {
	chartDir := findChartDir(t)
	doc, err := os.ReadFile(filepath.Join(chartDir, "profiles", "PACKAGING.md"))
	if err != nil {
		t.Fatalf("read PACKAGING.md: %v", err)
	}
	for _, p := range chartProfilePrograms() {
		if !containsStr(string(doc), p.rel+"/") {
			t.Errorf("PACKAGING.md does not document staged program %q (%s)", p.src, p.rel)
		}
	}
}

func containsStr(haystack, needle string) bool {
	return len(needle) > 0 && (len(haystack) >= len(needle)) && indexOf(haystack, needle) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
