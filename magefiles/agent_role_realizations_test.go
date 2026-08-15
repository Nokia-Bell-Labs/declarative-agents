// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const realizationModelPath = "../applications/docs/specs/semantic-models/agent-role-realizations.yaml"

type realizationModel struct {
	CanonicalRoles    []canonicalRole                  `yaml:"canonical_roles"`
	CanonicalSubRoles []canonicalSubRole               `yaml:"canonical_sub_roles"`
	Classifications   map[string]realizationClass      `yaml:"classifications"`
	Bindings          map[string][]realizationBinding  `yaml:"bindings"`
	NonRoleInventory  map[string]nonRoleInventoryGroup `yaml:"non_role_inventory"`
	MigrationAliases  []migrationAlias                 `yaml:"migration_aliases"`
}

type canonicalRole struct {
	ID   string `yaml:"id"`
	Name string `yaml:"name"`
}

type canonicalSubRole struct {
	ID     string `yaml:"id"`
	Parent string `yaml:"parent"`
}

type realizationClass struct {
	CountsTowardRoleCoverage bool `yaml:"counts_toward_role_coverage"`
}

type roleBinding struct {
	Role     string   `yaml:"role"`
	SubRoles []string `yaml:"sub_roles"`
}

type realizationBinding struct {
	Actor                   string        `yaml:"actor"`
	Profile                 string        `yaml:"profile"`
	Classification          string        `yaml:"classification"`
	PrimaryRole             string        `yaml:"primary_role"`
	SubRoles                []string      `yaml:"sub_roles"`
	Roles                   []roleBinding `yaml:"roles"`
	Inherits                string        `yaml:"inherits"`
	InheritanceTarget       string        `yaml:"inheritance_target"`
	ResponsibilityJustified string        `yaml:"responsibility_justification"`
	NamingStatus            string        `yaml:"naming_status"`
}

type nonRoleInventoryGroup struct {
	Profiles []yaml.Node `yaml:"profiles"`
}

type migrationAlias struct {
	Alias            string `yaml:"alias"`
	Path             string `yaml:"path"`
	Status           string `yaml:"status"`
	CollisionWith    string `yaml:"collision_with"`
	TargetName       string `yaml:"target_name"`
	CanonicalPath    string `yaml:"canonical_path"`
	SupportedThrough string `yaml:"supported_through"`
	Removal          string `yaml:"removal"`
}

type indexedBinding struct {
	Group string
	realizationBinding
}

func TestAgentRoleRealizationsRepositoryConformance(t *testing.T) {
	root := repositoryRoot(t)
	model := readRealizationModel(t)
	profiles, err := discoverRepositoryProfiles(root)
	if err != nil {
		t.Fatal(err)
	}
	if failures := validateRealizationModel(root, model, profiles); len(failures) != 0 {
		t.Fatalf("agent-role realization model violations:\n%s", strings.Join(failures, "\n"))
	}
}

func TestAgentRoleProfileDiscoveryExcludesGeneratedPackageTrees(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{
		"applications/example/agents/profile.yaml",
		"applications/example/build/profiles/agents/generated/profile.yaml",
		"applications/example/helm/profiles/agents/generated/profile.yaml",
		"applications/example/helm/dist/unpacked/profile.yaml",
		"applications/example/node_modules/fixture/profile.yaml",
	} {
		filename := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filename, []byte("name: fixture\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	profiles, err := discoverRepositoryProfiles(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"applications/example/agents/profile.yaml"}
	if !reflect.DeepEqual(profiles, want) {
		t.Fatalf("discovered profiles = %v, want source-only inventory %v", profiles, want)
	}
}

func TestAgentRoleRealizationFailureFixtures(t *testing.T) {
	root := repositoryRoot(t)
	profiles, err := discoverRepositoryProfiles(root)
	if err != nil {
		t.Fatal(err)
	}
	fixtures := []struct {
		name, want string
		mutate     func(*realizationModel)
	}{
		{"unknown role", "unknown canonical role", func(m *realizationModel) {
			m.Bindings["catalog"][0].PrimaryRole = "reviewer"
		}},
		{"incorrect sub-role parent", "belongs to planner, not critic", func(m *realizationModel) {
			m.Bindings["catalog"][0].SubRoles = []string{"intent-handler"}
		}},
		{"missing production classification", "missing classification", func(m *realizationModel) {
			m.Bindings["catalog"][0].Classification = ""
		}},
		{"stale profile path", "profile path does not exist", func(m *realizationModel) {
			m.Bindings["catalog"][0].Profile = "applications/catalog/agents/critic/stale-profile.yaml"
		}},
		{"incompatible canonical-name reuse", "bare canonical role name conflicts", func(m *realizationModel) {
			for i := range m.Bindings["catalog"] {
				if m.Bindings["catalog"][i].Actor == "assembler" {
					m.Bindings["catalog"][i].NamingStatus = ""
				}
			}
		}},
		{"unknown inheritance target", "unknown inheritance target", func(m *realizationModel) {
			m.Bindings["coding_agent"][0].Inherits = "missing-planner"
		}},
		{"inheritance cycle", "inheritance cycle", func(m *realizationModel) {
			m.Bindings["catalog"][0].Inherits = "critic-workspace"
		}},
		{"undocumented binding override", "redefines inherited roles without responsibility_justification", func(m *realizationModel) {
			m.Bindings["catalog"][1].PrimaryRole = "critic"
		}},
		{"coverage exclusion", "must not count toward role coverage", func(m *realizationModel) {
			class := m.Classifications["human_interface"]
			class.CountsTowardRoleCoverage = true
			m.Classifications["human_interface"] = class
		}},
		{"missing collision alias", "requires a matching migration alias", func(m *realizationModel) {
			m.MigrationAliases = m.MigrationAliases[1:]
		}},
		{"missing compatibility duration", "compatibility alias must define canonical_path, supported_through, and removal", func(m *realizationModel) {
			m.MigrationAliases[1].Removal = ""
		}},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			model := readRealizationModel(t)
			fixture.mutate(&model)
			failures := validateRealizationModel(root, model, profiles)
			if !containsFailure(failures, fixture.want) {
				t.Fatalf("failures = %q, want actionable violation containing %q", failures, fixture.want)
			}
		})
	}
}

func TestAgentRoleRealizationManyToManyFixtures(t *testing.T) {
	model := readRealizationModel(t)
	index := indexBindings(model)
	criticActors := map[string]bool{}
	for _, binding := range index {
		for role := range effectiveRoles(binding, index, nil, nil) {
			if role == "critic" {
				criticActors[binding.Actor] = true
			}
		}
	}
	if len(criticActors) < 5 {
		t.Fatalf("repeated Critic fixture has %d actors (%v), want at least 5", len(criticActors), criticActors)
	}
	chatbot := bindingByActor(index, "chatbot")
	if chatbot == nil || len(chatbot.Roles) != 3 {
		t.Fatalf("chatbot binding = %#v, want descriptive actor with 3 role bindings", chatbot)
	}
}

func TestAgentRoleRealizationMeshCritics(t *testing.T) {
	root := repositoryRoot(t)
	model := readRealizationModel(t)
	inventory := model.NonRoleInventory["mesh_fixtures"]
	byPath := map[string]nonRoleProfileEntry{}
	for _, node := range inventory.Profiles {
		entry, err := nonRoleProfile(node)
		if err != nil {
			t.Fatal(err)
		}
		byPath[entry.Path] = entry
	}

	tests := []struct {
		path, actor, scope string
		subRoles           []string
	}{
		{
			"applications/chatbot-mesh/agents/chatbot/tests/single-turn/profile.yaml",
			"chatbot-turn-critic", "single-turn-scenario",
			[]string{"output-evaluator", "sandbox-validator"},
		},
		{
			"applications/chatbot-mesh/agents/chatbot/tests/degraded-rag/profile.yaml",
			"chatbot-turn-critic", "degraded-rag-scenario",
			[]string{"output-evaluator"},
		},
		{
			"applications/chatbot-mesh/agents/rag-server/tests/query/profile.yaml",
			"rag-query-critic", "query-scenario",
			[]string{"output-evaluator"},
		},
	}
	for _, test := range tests {
		t.Run(test.scope, func(t *testing.T) {
			entry := byPath[test.path]
			if entry.Actor != test.actor || entry.Role != "critic" ||
				strings.Join(entry.SubRoles, ",") != strings.Join(test.subRoles, ",") ||
				entry.Domain == "" || entry.ExecutionScope != test.scope {
				t.Fatalf("mesh Critic binding = %#v, want actor %q, Critic / %v with domain and scope %q",
					entry, test.actor, test.subRoles, test.scope)
			}
			data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(test.path)))
			if err != nil {
				t.Fatal(err)
			}
			var profile struct {
				Name string `yaml:"name"`
			}
			if err := yaml.Unmarshal(data, &profile); err != nil {
				t.Fatal(err)
			}
			if profile.Name != entry.Actor {
				t.Fatalf("%s name = %q, want shared binding actor %q", test.path, profile.Name, entry.Actor)
			}
		})
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := findRepositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	return absolute
}

func readRealizationModel(t *testing.T) realizationModel {
	t.Helper()
	data, err := os.ReadFile(realizationModelPath)
	if err != nil {
		t.Fatalf("read %s: %v", realizationModelPath, err)
	}
	var model realizationModel
	if err := yaml.Unmarshal(data, &model); err != nil {
		t.Fatalf("parse %s: %v", realizationModelPath, err)
	}
	return model
}

func discoverRepositoryProfiles(root string) ([]string, error) {
	var profiles []string
	err := filepath.WalkDir(filepath.Join(root, "applications"), func(file string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, file)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if entry.IsDir() {
			if isGeneratedProfileTree(rel) {
				return filepath.SkipDir
			}
			return nil
		}
		if !isSemanticProfileFile(entry.Name()) {
			return nil
		}
		profiles = append(profiles, rel)
		return nil
	})
	sort.Strings(profiles)
	return profiles, err
}

func isGeneratedProfileTree(rel string) bool {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	for index, part := range parts {
		if part == "build" || part == "node_modules" {
			return true
		}
		if part == "helm" && index+1 < len(parts) &&
			(parts[index+1] == "dist" || parts[index+1] == "profiles") {
			return true
		}
	}
	return false
}

func isSemanticProfileFile(name string) bool {
	return name == "profile.yaml" ||
		(strings.HasPrefix(name, "profile-") && strings.HasSuffix(name, ".yaml")) ||
		strings.HasSuffix(name, "-profile.yaml")
}

func validateRealizationModel(root string, model realizationModel, discovered []string) []string {
	var failures []string
	fail := func(rule, location, detail string) {
		failures = append(failures, fmt.Sprintf("%s: %s: %s", location, rule, detail))
	}
	idPattern := regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	roles := map[string]bool{}
	roleNames := map[string]bool{}
	for _, role := range model.CanonicalRoles {
		if !idPattern.MatchString(role.ID) || roles[role.ID] {
			fail("canonical-role-id", role.ID, "role IDs must be unique lower-kebab-case")
		}
		if roleNames[strings.ToLower(role.Name)] {
			fail("canonical-role-name", role.ID, "canonical display names must be unique")
		}
		roles[role.ID], roleNames[strings.ToLower(role.Name)] = true, true
	}
	if len(model.CanonicalRoles) != 14 {
		fail("canonical-role-count", realizationModelPath, fmt.Sprintf("got %d roles, want 14", len(model.CanonicalRoles)))
	}
	subRoles := map[string]string{}
	for _, subRole := range model.CanonicalSubRoles {
		if !idPattern.MatchString(subRole.ID) || subRoles[subRole.ID] != "" {
			fail("canonical-sub-role-id", subRole.ID, "sub-role IDs must be unique lower-kebab-case")
		}
		if !roles[subRole.Parent] {
			fail("canonical-sub-role-parent", subRole.ID, "parent "+subRole.Parent+" is not a canonical role")
		}
		subRoles[subRole.ID] = subRole.Parent
	}
	if len(model.CanonicalSubRoles) != 41 {
		fail("canonical-sub-role-count", realizationModelPath, fmt.Sprintf("got %d sub-roles, want 41", len(model.CanonicalSubRoles)))
	}
	for _, excluded := range []string{
		"configuration_variant", "fixture", "human_interface", "infrastructure_adapter",
		"mock", "test_harness", "wrapper",
	} {
		class, ok := model.Classifications[excluded]
		if !ok {
			fail("classification", excluded, "required exclusion classification is missing")
		} else if class.CountsTowardRoleCoverage {
			fail("coverage-exclusion", excluded, "classification must not count toward role coverage")
		}
	}

	index := indexBindings(model)
	listedProfiles := map[string]string{}
	for i := range index {
		binding := index[i]
		location := fmt.Sprintf("%s binding %q (%s)", binding.Group, binding.Actor, binding.Profile)
		if binding.Classification == "" {
			fail("classification", location, "missing classification")
		} else if _, ok := model.Classifications[binding.Classification]; !ok {
			fail("classification", location, "unknown classification "+binding.Classification)
		}
		validateProfilePath(root, binding.Profile, location, listedProfiles, fail)
		validateDirectRoles(binding, roles, subRoles, fail)
		if (binding.Classification == "wrapper" || binding.Classification == "configuration_variant") && binding.Inherits == "" {
			fail("inheritance", location, binding.Classification+" must name an inheritance target")
		}
		if binding.Inherits != "" && (binding.PrimaryRole != "" || len(binding.Roles) != 0) && binding.ResponsibilityJustified == "" {
			fail("binding-override", location, "redefines inherited roles without responsibility_justification")
		}
	}
	for group, inventory := range model.NonRoleInventory {
		for i := range inventory.Profiles {
			entry, err := nonRoleProfile(inventory.Profiles[i])
			location := fmt.Sprintf("non_role_inventory.%s[%d]", group, i)
			if err != nil {
				fail("non-role-inventory", location, err.Error())
				continue
			}
			if entry.Classification != "" {
				class, ok := model.Classifications[entry.Classification]
				if !ok {
					fail("classification", location, "unknown classification "+entry.Classification)
				} else if class.CountsTowardRoleCoverage {
					fail("coverage-exclusion", location, entry.Classification+" must not count toward role coverage")
				}
			}
			if entry.Role != "" {
				binding := indexedBinding{
					Group: group,
					realizationBinding: realizationBinding{
						Profile:     entry.Path,
						PrimaryRole: entry.Role,
						SubRoles:    entry.SubRoles,
					},
				}
				validateDirectRoles(binding, roles, subRoles, fail)
			}
			validateProfilePath(root, entry.Path, location, listedProfiles, fail)
		}
	}
	for _, profile := range discovered {
		if _, ok := listedProfiles[profile]; !ok {
			fail("production-profile-inventory", profile, "profile missing classification or binding")
		}
	}

	for i := range index {
		binding := index[i]
		location := fmt.Sprintf("%s binding %q (%s)", binding.Group, binding.Actor, binding.Profile)
		stack := map[string]bool{}
		effective := effectiveRoles(binding, index, stack, &failures)
		if binding.Inherits == "" && (binding.Classification == "role_realization" ||
			binding.Classification == "descriptive_multi_role_actor") && len(effective) == 0 {
			fail("role-binding", location, "production realization has no canonical role binding")
		}
		if roles[binding.Actor] && !compatibleCanonicalName(binding, effective) &&
			binding.NamingStatus != "migration_required_collision" &&
			binding.NamingStatus != "boundary_review_required" {
			fail("canonical-name-collision", location,
				"bare canonical role name conflicts with documented responsibility; record migration or boundary-review status")
		}
	}
	validateAliases(model, roles, listedProfiles, fail)
	sort.Strings(failures)
	return failures
}

func indexBindings(model realizationModel) []indexedBinding {
	groups := make([]string, 0, len(model.Bindings))
	for group := range model.Bindings {
		groups = append(groups, group)
	}
	sort.Strings(groups)
	var index []indexedBinding
	for _, group := range groups {
		for _, binding := range model.Bindings[group] {
			index = append(index, indexedBinding{Group: group, realizationBinding: binding})
		}
	}
	return index
}

func validateDirectRoles(binding indexedBinding, roles map[string]bool, subRoles map[string]string,
	fail func(string, string, string)) {
	location := fmt.Sprintf("%s binding %q (%s)", binding.Group, binding.Actor, binding.Profile)
	check := func(role string, children []string) {
		if !roles[role] {
			fail("role-binding", location, "unknown canonical role "+role)
		}
		for _, child := range children {
			parent, ok := subRoles[child]
			if !ok {
				fail("sub-role-binding", location, "unknown canonical sub-role "+child)
			} else if parent != role {
				fail("sub-role-parent", location, fmt.Sprintf("%s belongs to %s, not %s", child, parent, role))
			}
		}
	}
	if binding.PrimaryRole != "" {
		check(binding.PrimaryRole, binding.SubRoles)
	}
	for _, role := range binding.Roles {
		check(role.Role, role.SubRoles)
	}
}

func effectiveRoles(binding indexedBinding, index []indexedBinding, stack map[string]bool, failures *[]string) map[string]bool {
	if stack == nil {
		stack = map[string]bool{}
	}
	key := binding.Profile
	if stack[key] {
		if failures != nil {
			*failures = append(*failures, fmt.Sprintf("%s binding %q (%s): inheritance: inheritance cycle at %s",
				binding.Group, binding.Actor, binding.Profile, binding.Actor))
		}
		return nil
	}
	stack[key] = true
	defer delete(stack, key)
	result := map[string]bool{}
	if binding.Inherits != "" {
		target := resolveInheritance(binding, index)
		if target == nil {
			if failures != nil {
				*failures = append(*failures, fmt.Sprintf("%s binding %q (%s): inheritance: unknown inheritance target %q",
					binding.Group, binding.Actor, binding.Profile, binding.Inherits))
			}
		} else {
			for role := range effectiveRoles(*target, index, stack, failures) {
				result[role] = true
			}
		}
	}
	if binding.PrimaryRole != "" {
		result[binding.PrimaryRole] = true
	}
	for _, role := range binding.Roles {
		result[role.Role] = true
	}
	return result
}

func resolveInheritance(binding indexedBinding, index []indexedBinding) *indexedBinding {
	var matches []int
	for i := range index {
		if binding.InheritanceTarget != "" {
			if index[i].Profile == binding.InheritanceTarget {
				matches = append(matches, i)
			}
		} else if index[i].Actor == binding.Inherits {
			matches = append(matches, i)
		}
	}
	if len(matches) != 1 {
		return nil
	}
	return &index[matches[0]]
}

func compatibleCanonicalName(binding indexedBinding, effective map[string]bool) bool {
	return binding.Classification == "role_realization" && effective[binding.Actor] &&
		binding.NamingStatus != "migration_required_collision"
}

func validateAliases(model realizationModel, roles map[string]bool, listed map[string]string,
	fail func(string, string, string)) {
	aliases := map[string]bool{}
	collisions := map[string]string{}
	actorsByProfile := map[string]string{}
	for _, bindings := range model.Bindings {
		for _, binding := range bindings {
			actorsByProfile[binding.Profile] = binding.Actor
		}
	}
	for _, alias := range model.MigrationAliases {
		location := fmt.Sprintf("migration alias %q (%s)", alias.Alias, alias.Path)
		if aliases[alias.Alias] {
			fail("alias", location, "duplicate alias")
		}
		aliases[alias.Alias] = true
		targetPath := alias.Path
		if alias.CanonicalPath != "" {
			targetPath = alias.CanonicalPath
		}
		if _, ok := listed[targetPath]; !ok {
			fail("alias", location, "alias target does not resolve to a classified profile")
		}
		if alias.CanonicalPath != "" && actorsByProfile[alias.CanonicalPath] != alias.TargetName {
			fail("alias-target", location, "target_name must match the canonical profile actor")
		}
		if alias.CollisionWith != "" && !roles[alias.CollisionWith] {
			fail("alias", location, "collision_with is not a canonical role")
		}
		if alias.CollisionWith != "" {
			collisions[alias.Path] = alias.CollisionWith
			if alias.CanonicalPath != "" {
				collisions[alias.CanonicalPath] = alias.CollisionWith
			}
		}
		if roles[alias.TargetName] {
			fail("alias", location, "target_name must not reuse a canonical role ID")
		}
		if !strings.HasPrefix(alias.Status, "migration_") {
			fail("alias", location, "status must be an explicit migration state")
		}
		if alias.Status == "migration_compatibility" &&
			(alias.CanonicalPath == "" || alias.SupportedThrough == "" || alias.Removal == "") {
			fail("alias-duration", location,
				"compatibility alias must define canonical_path, supported_through, and removal")
		}
	}
	for _, bindings := range model.Bindings {
		for _, binding := range bindings {
			if binding.NamingStatus == "migration_required_collision" &&
				collisions[binding.Profile] != binding.Actor {
				fail("alias", binding.Profile, "canonical-name collision requires a matching migration alias")
			}
		}
	}
}

func validateProfilePath(root, profile, location string, listed map[string]string,
	fail func(string, string, string)) {
	clean := path.Clean(profile)
	if profile == "" || clean != profile || path.IsAbs(profile) || !strings.HasPrefix(profile, "applications/") {
		fail("profile-path", location, "profile path must be clean, repository-relative, and under applications/")
		return
	}
	if previous, duplicate := listed[profile]; duplicate {
		fail("profile-inventory", location, "profile is already classified by "+previous)
	}
	listed[profile] = location
	info, err := os.Stat(filepath.Join(root, filepath.FromSlash(profile)))
	if err != nil || info.IsDir() {
		fail("profile-path", location, "profile path does not exist: "+profile)
	}
}

type nonRoleProfileEntry struct {
	Path                    string   `yaml:"path"`
	Actor                   string   `yaml:"actor"`
	Classification          string   `yaml:"classification"`
	Role                    string   `yaml:"role"`
	SubRoles                []string `yaml:"sub_roles"`
	Domain                  string   `yaml:"domain"`
	ExecutionScope          string   `yaml:"execution_scope"`
	ResponsibilityJustified string   `yaml:"responsibility_justification"`
}

func nonRoleProfile(node yaml.Node) (nonRoleProfileEntry, error) {
	switch node.Kind {
	case yaml.ScalarNode:
		return nonRoleProfileEntry{Path: node.Value, Classification: "fixture"}, nil
	case yaml.MappingNode:
		var entry nonRoleProfileEntry
		if err := node.Decode(&entry); err != nil {
			return nonRoleProfileEntry{}, err
		}
		if entry.Classification == "" {
			entry.Classification = "fixture"
		}
		return entry, nil
	default:
		return nonRoleProfileEntry{}, fmt.Errorf("profile entry must be a path or mapping")
	}
}

func bindingByActor(index []indexedBinding, actor string) *indexedBinding {
	for i := range index {
		if index[i].Actor == actor {
			return &index[i]
		}
	}
	return nil
}

func containsFailure(failures []string, want string) bool {
	for _, failure := range failures {
		if strings.Contains(failure, want) {
			return true
		}
	}
	return false
}
