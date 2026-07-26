// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"os"
	"strings"
	"testing"
)

func TestAssemblerAndMockAreSupportedTestTimeMembers(t *testing.T) {
	t.Parallel()
	type entry struct {
		ID      string `yaml:"id"`
		Path    string `yaml:"path"`
		Support string `yaml:"support"`
	}
	var index struct {
		SRDs []entry `yaml:"srd_index"`
	}
	readRoleYAML(t, "docs/SPECIFICATIONS.yaml", &index)

	wanted := map[string]string{
		"srd018-assembler": "agents/assembler/profile.yaml",
		"srd019-mock":      "agents/mock/profile.yaml",
	}
	for _, srd := range index.SRDs {
		profile, ok := wanted[srd.ID]
		if !ok {
			continue
		}
		if srd.Support != "supported-test-time" {
			t.Errorf("%s support = %q, want supported-test-time", srd.ID, srd.Support)
		}
		if _, err := os.Stat(ProfilePath(srd.Path)); err != nil {
			t.Errorf("%s SRD path: %v", srd.ID, err)
		}
		if _, err := os.Stat(ProfilePath(profile)); err != nil {
			t.Errorf("%s profile path: %v", srd.ID, err)
		}
		delete(wanted, srd.ID)
	}
	if len(wanted) != 0 {
		t.Errorf("missing supported test-time family records: %v", wanted)
	}
}

func TestMembershipNarrativeSeparatesMembersFromFixtures(t *testing.T) {
	t.Parallel()
	for _, rel := range []string{"README.md", "AGENTS.md", "docs/ARCHITECTURE.yaml", "docs/SPECIFICATIONS.yaml"} {
		data, err := os.ReadFile(ProfilePath(rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		text := strings.Join(strings.Fields(strings.ToLower(string(data))), " ")
		if !strings.Contains(text, "assembler and mock") ||
			!strings.Contains(text, "supported test-time library member") {
			t.Errorf("%s does not classify assembler and mock as supported test-time members", rel)
		}
		if !strings.Contains(text, "rig-subject") ||
			!strings.Contains(text, "internal") {
			t.Errorf("%s does not preserve the internal rig-subject fixture boundary", rel)
		}
	}
}
