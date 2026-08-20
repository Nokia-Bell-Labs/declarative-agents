// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

var relocatedMeshRecords = map[string][]string{
	"docs/specs/software-requirements/srd013-rag-server-agent.yaml": {
		"problem", "goals", "requirements", "non_goals", "acceptance_criteria",
	},
	"docs/specs/software-requirements/srd014-chatbot-agent.yaml": {
		"problem", "goals", "requirements", "non_goals", "acceptance_criteria",
	},
	"docs/specs/software-requirements/srd015-chatbot-deployment.yaml": {
		"problem", "goals", "requirements", "non_goals", "acceptance_criteria",
	},
	"docs/specs/software-requirements/srd016-coordinator.yaml": {
		"problem", "goals", "requirements", "non_goals", "acceptance_criteria",
	},
	"docs/specs/software-requirements/srd017-creator.yaml": {
		"problem", "goals", "requirements", "non_goals", "acceptance_criteria",
	},
	"docs/specs/use-cases/rel09.0-uc001-rag-server-query.yaml": {
		"summary", "actor", "trigger", "flow", "touchpoints", "success_criteria", "out_of_scope",
	},
	"docs/specs/use-cases/rel09.0-uc002-chatbot-turn.yaml": {
		"summary", "actor", "trigger", "flow", "touchpoints", "success_criteria", "out_of_scope",
	},
	"docs/specs/use-cases/rel09.1-uc001-routed-fanout.yaml": {
		"summary", "actor", "trigger", "flow", "touchpoints", "success_criteria", "out_of_scope",
	},
	"docs/specs/use-cases/rel09.2-uc001-observability.yaml": {
		"summary", "actor", "trigger", "flow", "touchpoints", "success_criteria", "out_of_scope",
	},
	"docs/specs/use-cases/rel09.3-uc001-mesh-deployment.yaml": {
		"summary", "actor", "trigger", "flow", "touchpoints", "success_criteria", "out_of_scope",
	},
	"docs/specs/use-cases/rel09.4-uc001-control-plane-provisioning.yaml": {
		"summary", "actor", "trigger", "flow", "touchpoints", "success_criteria", "out_of_scope",
	},
	"docs/specs/use-cases/rel09.5-uc001-in-cluster-llm-tier.yaml": {
		"summary", "actor", "trigger", "flow", "touchpoints", "success_criteria", "out_of_scope",
	},
	"docs/specs/test-suites/test-rel09.0-chatbot-mesh.yaml": {
		"traces", "preconditions", "test_cases",
	},
	"docs/specs/test-suites/test-rel09.1-routing-fanout.yaml": {
		"traces", "preconditions", "test_cases",
	},
	"docs/specs/test-suites/test-rel09.2-observability.yaml": {
		"traces", "preconditions", "test_cases",
	},
	"docs/specs/test-suites/test-rel09.3-mesh-deployment.yaml": {
		"traces", "preconditions", "test_cases",
	},
	"docs/specs/test-suites/test-rel09.4-control-plane.yaml": {
		"traces", "preconditions", "test_cases",
	},
	"docs/specs/test-suites/test-rel09.5-llm-tier.yaml": {
		"traces", "preconditions", "test_cases",
	},
}

func TestRelocatedMeshRecordsArePointerOnlyTombstones(t *testing.T) {
	t.Parallel()
	pointerFields := map[string]bool{
		"id": true, "title": true, "release": true, "status": true,
		"canonical_path": true, "note": true,
	}
	for rel, required := range relocatedMeshRecords {
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
		allowed := make(map[string]bool, len(pointerFields)+len(required))
		for key := range pointerFields {
			allowed[key] = true
		}
		for _, key := range required {
			allowed[key] = true
		}
		for key, value := range record {
			if !allowed[key] {
				t.Errorf("%s contains live %q content; relocated records must be pointer-only", rel, key)
			} else if !pointerFields[key] && !isPointerSentinel(value) {
				t.Errorf("%s field %q = %#v, want empty or canonical_path sentinel", rel, key, value)
			}
		}
	}
}

func isPointerSentinel(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		return typed == "See canonical_path."
	case []any:
		return len(typed) == 0
	case map[string]any:
		return len(typed) == 0
	default:
		return false
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
	wanted := make(map[string]bool, len(relocatedMeshRecords))
	for rel := range relocatedMeshRecords {
		base := filepath.Base(rel)
		wanted[strings.TrimSuffix(base, filepath.Ext(base))] = false
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
