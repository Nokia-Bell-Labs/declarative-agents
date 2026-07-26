// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestCurrentRoleContractsUseCanonicalNames(t *testing.T) {
	t.Parallel()
	for _, rel := range []string{
		"docs/specs/software-requirements/srd001-agent-functional-blocks.yaml",
		"docs/specs/software-requirements/srd004-planner.yaml",
		"docs/specs/software-requirements/srd006-bench.yaml",
	} {
		data, err := os.ReadFile(ProfilePath(rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		lower := strings.ToLower(string(data))
		for _, legacy := range []string{"generator", "evaluator"} {
			if strings.Contains(lower, legacy) {
				t.Errorf("%s contains legacy current-role name %q", rel, legacy)
			}
		}
	}
}

func TestCanonicalRoleIndexTitlesMatchSourceSRDs(t *testing.T) {
	t.Parallel()
	var index struct {
		SRDs []struct {
			ID    string `yaml:"id"`
			Title string `yaml:"title"`
			Path  string `yaml:"path"`
		} `yaml:"srd_index"`
	}
	readRoleYAML(t, "docs/SPECIFICATIONS.yaml", &index)

	wanted := map[string]bool{"srd002-executor": false, "srd003-critic": false}
	for _, entry := range index.SRDs {
		if _, ok := wanted[entry.ID]; !ok {
			continue
		}
		var source struct {
			Title string `yaml:"title"`
		}
		readRoleYAML(t, entry.Path, &source)
		if entry.Title != source.Title {
			t.Errorf("%s index title = %q, source title = %q", entry.ID, entry.Title, source.Title)
		}
		wanted[entry.ID] = true
	}
	for id, found := range wanted {
		if !found {
			t.Errorf("SPECIFICATIONS srd_index missing %s", id)
		}
	}
}

func readRoleYAML(t *testing.T, rel string, out any) {
	t.Helper()
	data, err := os.ReadFile(ProfilePath(rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	if err := yaml.Unmarshal(data, out); err != nil {
		t.Fatalf("parse %s: %v", rel, err)
	}
}
