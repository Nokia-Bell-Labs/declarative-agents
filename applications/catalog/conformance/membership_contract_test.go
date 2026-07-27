// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestScenarioCriticAndMockAreSupportedTestTimeMembers(t *testing.T) {
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
		"srd018-scenario-critic": "agents/scenario-critic/profile.yaml",
		"srd019-mock":            "agents/mock/profile.yaml",
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
		if !strings.Contains(text, "scenario-critic and mock") ||
			!strings.Contains(text, "supported test-time library member") {
			t.Errorf("%s does not classify scenario-critic and mock as supported test-time members", rel)
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
		Actor          string   `yaml:"actor"`
		Profile        string   `yaml:"profile"`
		Classification string   `yaml:"classification"`
		PrimaryRole    string   `yaml:"primary_role"`
		SubRoles       []string `yaml:"sub_roles"`
		Inherits       string   `yaml:"inherits"`
		NamingStatus   string   `yaml:"naming_status"`
	}
	var authority struct {
		Bindings map[string][]binding `yaml:"bindings"`
		Aliases  []struct {
			Alias            string `yaml:"alias"`
			Path             string `yaml:"path"`
			Status           string `yaml:"status"`
			CollisionWith    string `yaml:"collision_with"`
			TargetName       string `yaml:"target_name"`
			CanonicalPath    string `yaml:"canonical_path"`
			SupportedThrough string `yaml:"supported_through"`
			Removal          string `yaml:"removal"`
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
	if got := byActor["scenario-critic"]; got.Classification != "test_harness" ||
		got.PrimaryRole != "critic" ||
		strings.Join(got.SubRoles, ",") != "sandbox-validator,output-evaluator" ||
		got.NamingStatus != "" {
		t.Errorf("scenario-critic shared binding = %#v, want test-harness Critic / Sandbox Validator / Output Evaluator", got)
	}
	if got := byActor["assembler"]; got.Classification != "wrapper" ||
		got.Inherits != "scenario-critic" || got.NamingStatus != "migration_required_collision" {
		t.Errorf("assembler compatibility binding = %#v, want wrapper inheriting scenario-critic", got)
	}
	if got := byActor["mock"]; got.Classification != "mock" || got.PrimaryRole != "" {
		t.Errorf("mock shared binding = %#v, want unbound mock classification", got)
	}
	if got := byActor["runtime-state-reader"]; got.Classification != "infrastructure_adapter" ||
		got.Profile != "applications/catalog/agents/runtime-state-reader/profile.yaml" ||
		got.NamingStatus != "" {
		t.Errorf("runtime-state-reader shared binding = %#v, want named infrastructure adapter", got)
	}
	if got := byActor["specification-critic"]; got.Classification != "role_realization" ||
		got.Profile != "applications/catalog/agents/specification-critic/profile.yaml" ||
		got.PrimaryRole != "critic" ||
		strings.Join(got.SubRoles, ",") != "output-evaluator" ||
		got.NamingStatus != "" {
		t.Errorf("specification-critic shared binding = %#v, want Critic / Output Evaluator without migration status", got)
	}
	aliases := map[string]string{}
	for _, alias := range authority.Aliases {
		if !strings.HasPrefix(alias.Status, "migration_") {
			t.Errorf("alias %s has non-migration status %q", alias.Alias, alias.Status)
		}
		if alias.CollisionWith != "" && alias.CollisionWith == alias.TargetName {
			t.Errorf("alias %s normalizes its collision as target %q", alias.Alias, alias.TargetName)
		}
		aliases[alias.Path] = alias.TargetName
	}
	for path, target := range map[string]string{
		"applications/catalog/agents/assembler/profile.yaml":    "scenario-critic",
		"applications/catalog/agents/" + "monitor/profile.yaml": "runtime-state-reader",
		"applications/catalog/agents/jurist/profile.yaml":       "specification-critic",
	} {
		if aliases[path] != target {
			t.Errorf("migration alias %s = %q, want %q", path, aliases[path], target)
		}
	}
	compatibilityTargets := map[string]string{
		"assembler": "applications/catalog/agents/scenario-critic/profile.yaml",
		"monitor":   "applications/catalog/agents/runtime-state-reader/profile.yaml",
		"jurist":    "applications/catalog/agents/specification-critic/profile.yaml",
	}
	for _, alias := range authority.Aliases {
		canonicalPath, ok := compatibilityTargets[alias.Alias]
		if !ok {
			continue
		}
		if alias.Status != "migration_compatibility" ||
			alias.CanonicalPath != canonicalPath ||
			alias.SupportedThrough != "applications/catalog/v0.*" ||
			alias.Removal != "applications/catalog/v1" {
			t.Errorf("%s compatibility duration/target = %#v", alias.Alias, alias)
		}
	}

	var wrapper struct {
		Name             string   `yaml:"name"`
		Machine          string   `yaml:"machine"`
		Tools            []string `yaml:"tools"`
		ToolDeclarations []string `yaml:"tool_declarations"`
		RESTDefinitions  []string `yaml:"rest_definitions"`
	}
	readRoleYAML(t, "agents/assembler/profile.yaml", &wrapper)
	if wrapper.Name != "assembler" ||
		wrapper.Machine != "../scenario-critic/machine.yaml" ||
		strings.Join(wrapper.Tools, ",") != "../scenario-critic/tools.yaml" ||
		strings.Join(wrapper.ToolDeclarations, ",") != "../scenario-critic/declarations.yaml" ||
		strings.Join(wrapper.RESTDefinitions, ",") != "../scenario-critic/rest.yaml" {
		t.Errorf("assembler compatibility profile forks or misses canonical scenario-critic closure: %#v", wrapper)
	}
	entries, err := os.ReadDir(ProfilePath("agents/assembler"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "profile.yaml" {
		t.Errorf("assembler compatibility directory must contain only profile.yaml, got %v", entries)
	}
}

func TestNoActiveAssemblerPathConsumers(t *testing.T) {
	t.Parallel()
	const retiredPath = "agents/" + "assembler"
	roots := []string{ProfilesRoot(), ProfilePath("../chatbot-mesh")}
	allowed := map[string]int{
		filepath.Clean(ProfilePath("conformance/membership_contract_test.go")): 3,
		filepath.Clean(ProfilePath("README.md")):                               1,
		filepath.Clean(ProfilePath("docs/SPECIFICATIONS.yaml")):                1,
		filepath.Clean(ProfilePath("docs/road-map.yaml")):                      1,
	}
	seen := map[string]int{}
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			switch filepath.Ext(path) {
			case ".go", ".md", ".yaml", ".yml":
			default:
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if count := strings.Count(string(data), retiredPath); count != 0 {
				seen[filepath.Clean(path)] = count
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for path, count := range seen {
		if allowed[path] != count {
			t.Errorf("stale active consumer path %s appears %d times in %s", retiredPath, count, path)
		}
	}
	for path, count := range allowed {
		if seen[path] != count {
			t.Errorf("historical compatibility allowlist drift for %s: got %d occurrences, want %d", path, seen[path], count)
		}
	}
}

func TestNoActiveMonitorProfilePathConsumers(t *testing.T) {
	t.Parallel()
	retiredPath := "agents/" + "monitor"
	roots := []string{
		ProfilesRoot(),
		filepath.Clean(filepath.Join(ProfilesRoot(), "..", "..", "agent-core")),
	}
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			switch filepath.Ext(path) {
			case ".go", ".md", ".yaml", ".yml":
			default:
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if strings.Contains(string(data), retiredPath) {
				t.Errorf("stale active external profile path %s in %s", retiredPath, path)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestNoActiveJuristProfilePathConsumers(t *testing.T) {
	t.Parallel()
	retiredPath := "agents/" + "jurist"
	roots := []string{
		ProfilesRoot(),
		filepath.Clean(filepath.Join(ProfilesRoot(), "..", "docs")),
		filepath.Clean(filepath.Join(ProfilesRoot(), "..", "chatbot-mesh")),
		filepath.Clean(filepath.Join(ProfilesRoot(), "..", "..", "agent-core")),
		filepath.Clean(filepath.Join(ProfilesRoot(), "..", "..", "design-patterns")),
	}
	allowed := map[string]int{
		filepath.Clean(ProfilePath("conformance/membership_contract_test.go")):                                                  1,
		filepath.Clean(filepath.Join(ProfilesRoot(), "..", "docs", "specs", "semantic-models", "agent-role-realizations.yaml")): 1,
	}
	seen := map[string]int{}
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			switch filepath.Ext(path) {
			case ".go", ".md", ".yaml", ".yml":
			default:
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if count := strings.Count(string(data), retiredPath); count != 0 {
				seen[filepath.Clean(path)] = count
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for path, count := range seen {
		if allowed[path] != count {
			t.Errorf("stale active consumer path %s appears %d times in %s", retiredPath, count, path)
		}
	}
	for path, count := range allowed {
		if seen[path] != count {
			t.Errorf("historical compatibility allowlist drift for %s: got %d occurrences, want %d", path, seen[path], count)
		}
	}
}
