// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package conformance

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestRelocatedMeshRecordsArePointerOnlyTombstones(t *testing.T) {
	t.Parallel()
	records := []string{
		"docs/specs/software-requirements/srd013-rag-server-agent.yaml",
		"docs/specs/software-requirements/srd014-chatbot-agent.yaml",
		"docs/specs/use-cases/rel09.0-uc001-rag-server-query.yaml",
		"docs/specs/use-cases/rel09.0-uc002-chatbot-turn.yaml",
		"docs/specs/use-cases/rel09.1-uc001-routed-fanout.yaml",
		"docs/specs/use-cases/rel09.2-uc001-observability.yaml",
		"docs/specs/test-suites/test-rel09.0-chatbot-mesh.yaml",
		"docs/specs/test-suites/test-rel09.1-routing-fanout.yaml",
		"docs/specs/test-suites/test-rel09.2-observability.yaml",
	}
	allowed := map[string]bool{
		"id": true, "title": true, "release": true, "status": true,
		"canonical_path": true, "note": true,
	}
	for _, rel := range records {
		var record map[string]any
		readRoleYAML(t, rel, &record)
		if record["status"] != "relocated" {
			t.Errorf("%s status = %v, want relocated", rel, record["status"])
		}
		canonical, ok := record["canonical_path"].(string)
		if !ok || canonical == "" {
			t.Errorf("%s has no canonical_path", rel)
		} else if _, err := os.Stat(filepath.Join(ProfilesRoot(), "..", "..", filepath.FromSlash(canonical))); err != nil {
			t.Errorf("%s canonical_path %s: %v", rel, canonical, err)
		}
		for key := range record {
			if !allowed[key] {
				t.Errorf("%s contains live %q content; relocated records must be pointer-only", rel, key)
			}
		}
	}
}

func TestRelocatedMeshSpecificationIndexPointsToCanonicalRecords(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(ProfilePath("docs/SPECIFICATIONS.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	type entry struct {
		ID            string `yaml:"id"`
		Status        string `yaml:"status"`
		CanonicalPath string `yaml:"canonical_path"`
	}
	var index struct {
		SRDs       []entry `yaml:"srd_index"`
		UseCases   []entry `yaml:"use_case_index"`
		TestSuites []entry `yaml:"test_suite_index"`
	}
	if err := yaml.Unmarshal(data, &index); err != nil {
		t.Fatal(err)
	}
	wanted := map[string]bool{
		"srd013-rag-server-agent": false, "srd014-chatbot-agent": false,
		"rel09.0-uc001-rag-server-query": false, "rel09.0-uc002-chatbot-turn": false,
		"rel09.1-uc001-routed-fanout": false, "rel09.2-uc001-observability": false,
		"test-rel09.0-chatbot-mesh": false, "test-rel09.1-routing-fanout": false,
		"test-rel09.2-observability": false,
	}
	for _, section := range [][]entry{index.SRDs, index.UseCases, index.TestSuites} {
		for _, indexed := range section {
			if _, ok := wanted[indexed.ID]; !ok {
				continue
			}
			if indexed.Status != "relocated" || indexed.CanonicalPath == "" {
				t.Errorf("%s index entry = status %q canonical_path %q", indexed.ID, indexed.Status, indexed.CanonicalPath)
			}
			wanted[indexed.ID] = true
		}
	}
	for id, found := range wanted {
		if !found {
			t.Errorf("SPECIFICATIONS missing relocated pointer for %s", id)
		}
	}
}
