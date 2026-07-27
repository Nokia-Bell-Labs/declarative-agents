// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
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

func TestCatalogMembershipUsesSharedRealizationAndAliasAuthority(t *testing.T) {
	t.Parallel()
	type binding struct {
		Actor          string `yaml:"actor"`
		Profile        string `yaml:"profile"`
		Classification string `yaml:"classification"`
		PrimaryRole    string `yaml:"primary_role"`
		NamingStatus   string `yaml:"naming_status"`
	}
	var authority struct {
		Bindings map[string][]binding `yaml:"bindings"`
		Aliases  []struct {
			Alias         string `yaml:"alias"`
			Path          string `yaml:"path"`
			Status        string `yaml:"status"`
			CollisionWith string `yaml:"collision_with"`
			TargetName    string `yaml:"target_name"`
		} `yaml:"migration_aliases"`
	}
	modelPath := filepath.Join(ProfilesRoot(), "..", "docs", "specs", "semantic-models", "agent-role-realizations.yaml")
	data, err := os.ReadFile(modelPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(data, &authority); err != nil {
		t.Fatalf("parse %s: %v", modelPath, err)
	}
	byActor := map[string]binding{}
	for _, item := range authority.Bindings["catalog"] {
		byActor[item.Actor] = item
	}
	if got := byActor["assembler"]; got.Classification != "test_harness" ||
		got.PrimaryRole != "critic" || got.NamingStatus != "migration_required_collision" {
		t.Errorf("assembler shared binding = %#v, want test-harness Critic collision", got)
	}
	if got := byActor["mock"]; got.Classification != "mock" || got.PrimaryRole != "" {
		t.Errorf("mock shared binding = %#v, want unbound mock classification", got)
	}
	if got := byActor["monitor"]; got.Classification != "infrastructure_adapter" ||
		got.NamingStatus != "migration_required_collision" {
		t.Errorf("monitor shared binding = %#v, want infrastructure-adapter collision", got)
	}
	aliases := map[string]string{}
	for _, alias := range authority.Aliases {
		if alias.Status != "migration_required" && alias.Status != "migration_planned" {
			t.Errorf("alias %s has non-migration status %q", alias.Alias, alias.Status)
		}
		if alias.CollisionWith != "" && alias.CollisionWith == alias.TargetName {
			t.Errorf("alias %s normalizes its collision as target %q", alias.Alias, alias.TargetName)
		}
		aliases[alias.Path] = alias.TargetName
	}
	for path, target := range map[string]string{
		"applications/catalog/agents/assembler/profile.yaml": "scenario-critic",
		"applications/catalog/agents/monitor/profile.yaml":   "runtime-state-reader",
		"applications/catalog/agents/jurist/profile.yaml":    "specification-critic",
	} {
		if aliases[path] != target {
			t.Errorf("migration alias %s = %q, want %q", path, aliases[path], target)
		}
	}
}
