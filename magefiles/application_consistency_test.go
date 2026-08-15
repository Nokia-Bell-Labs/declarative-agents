// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/appmanifest"
	"gopkg.in/yaml.v3"
)

var release14Applications = []string{
	"chatbot-mesh",
	"coding-agent",
	"agent-architecture",
}

var release14Capabilities = []string{
	"runnable_module",
	"managed_service",
	"packaged",
	"helm_managed",
	"kind_demo",
	"ui",
}

func TestApplicationConsistencyManifests(t *testing.T) {
	wantOwnership := map[string]string{
		"chatbot-mesh":       "agent-owning",
		"coding-agent":       "composition-only",
		"agent-architecture": "composition-only",
	}
	for _, application := range release14Applications {
		t.Run(application, func(t *testing.T) {
			manifest := loadRelease14Manifest(t, application)
			if manifest.Ownership != wantOwnership[application] {
				t.Errorf("ownership = %q, want %q", manifest.Ownership, wantOwnership[application])
			}
			if manifest.ModuleStatus != "implemented" {
				t.Errorf("module_status = %q, want implemented", manifest.ModuleStatus)
			}
			if len(manifest.Capabilities) != len(release14Capabilities) {
				t.Errorf("capabilities = %d, want the complete Release 14 set %v",
					len(manifest.Capabilities), release14Capabilities)
			}
			for _, name := range release14Capabilities {
				capability, exists := manifest.Capabilities[name]
				if !exists {
					t.Errorf("missing capability %s", name)
					continue
				}
				if (capability.Status == "implemented" || capability.Status == "partial" ||
					capability.Status == "dependency_gated") && len(capability.Evidence) == 0 {
					t.Errorf("capability %s status %s has no evidence", name, capability.Status)
				}
			}
			if len(manifest.Deployment.Entries) > 0 &&
				manifest.Capabilities["runnable_module"].Status != "implemented" {
				t.Error("deployment entries exist without an implemented runnable module")
			}
			if len(manifest.UI.Assets) > 0 &&
				manifest.Capabilities["ui"].Status == "not_applicable" {
				t.Error("UI assets exist while ui is not_applicable")
			}
			if len(manifest.UI.Assets) == 0 &&
				manifest.Capabilities["ui"].Status != "not_applicable" {
				t.Error("active UI capability has no manifest assets")
			}

			wantLocal := discoverPrimaryLocalProfiles(t, release14ApplicationRoot(application))
			var gotLocal []string
			for _, root := range manifest.Roots {
				if root.Ownership == "local" {
					gotLocal = append(gotLocal, root.Source)
				}
				if root.Ownership == "catalog" && root.CompatibleRelease == "" {
					t.Errorf("catalog root %s has no compatible_release", root.ID)
				}
			}
			sort.Strings(gotLocal)
			if !reflect.DeepEqual(gotLocal, wantLocal) {
				t.Errorf("manifest local roots = %v, discovered production roots = %v",
					gotLocal, wantLocal)
			}
		})
	}
}

func TestApplicationActorGrammar(t *testing.T) {
	profileName := regexp.MustCompile(`^(?:profile|[a-z0-9]+(?:-[a-z0-9]+)*-profile)\.yaml$`)
	for _, application := range release14Applications {
		t.Run(application, func(t *testing.T) {
			appRoot := release14ApplicationRoot(application)
			manifest := loadRelease14Manifest(t, application)
			inventory, err := appmanifest.Resolve(manifest, release14ManifestOptions(application))
			if err != nil {
				t.Fatal(err)
			}
			resolver := newRelease14ReferenceResolver(appRoot, release14CatalogRoot(), inventory)
			for _, root := range manifest.Roots {
				if strings.Contains(root.Source, "/tests/") {
					t.Errorf("fixture root %s entered production composition", root.Source)
				}
			}
			for _, entry := range manifest.Deployment.Entries {
				if strings.Contains(entry.ProfilePath, "/tests/") {
					t.Errorf("fixture profile %s entered deployment", entry.ProfilePath)
				}
			}
			err = filepath.WalkDir(filepath.Join(appRoot, "agents"),
				func(filename string, entry fs.DirEntry, walkErr error) error {
					if walkErr != nil {
						return walkErr
					}
					if entry.IsDir() {
						if entry.Name() == "tests" {
							return filepath.SkipDir
						}
						return nil
					}
					if !strings.HasSuffix(entry.Name(), "profile.yaml") &&
						entry.Name() != "profile.yaml" {
						return nil
					}
					if !profileName.MatchString(entry.Name()) {
						t.Errorf("%s has a non-normalized profile name", relativeToRepo(filename))
						return nil
					}
					validateRelease14Profile(t, filename, resolver)
					return nil
				})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestResolveRelease14ReferenceUsesDeclaredRuntimeMappings(t *testing.T) {
	resolver, profile, catalogTarget := release14ReferenceResolverFixture(t)

	resolved, external, err := resolver.resolve(profile, "../../../agents/library/default.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if resolved != catalogTarget || !external {
		t.Fatalf("relocated reference = (%q, %t), want (%q, true)",
			resolved, external, catalogTarget)
	}

	resolved, external, err = resolver.resolve(profile, "machine.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if resolved != filepath.Join(filepath.Dir(profile), "machine.yaml") || external {
		t.Fatalf("source-relative reference = (%q, %t)", resolved, external)
	}

	if _, external, err = resolver.resolve(profile, "/opt/agent-core/tools/builtin/llm"); err != nil || !external {
		t.Fatalf("runtime-owned reference = (external %t, error %v)", external, err)
	}
}

func TestResolveRelease14ReferenceRejectsUnsafeReferences(t *testing.T) {
	resolver, profile, _ := release14ReferenceResolverFixture(t)
	link := filepath.Join(filepath.Dir(profile), "linked-machine.yaml")
	if err := os.Symlink("machine.yaml", link); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name, reference, want string
	}{
		{"undeclared traversal", "../../../agents/undeclared/default.yaml", "outside declared runtime roots"},
		{"glob", "configs/*.yaml", "unbounded glob"},
		{"runtime glob", "/opt/agent-core/tools/*.yaml", "unbounded glob"},
		{"absolute", "/tmp/machine.yaml", "disallowed absolute"},
		{"non-runtime opt", "/opt/other/machine.yaml", "disallowed absolute"},
		{"opt traversal", "/opt/agent-core/../other/machine.yaml", "disallowed absolute"},
		{"windows absolute", "C:/tmp/machine.yaml", "disallowed absolute"},
		{"symlink", "linked-machine.yaml", "symlink"},
		{"dangling", "missing.yaml", "dangling reference"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, _, err := resolver.resolve(profile, test.reference); err == nil ||
				!strings.Contains(err.Error(), test.want) {
				t.Fatalf("resolve(%q) error = %v, want %q", test.reference, err, test.want)
			}
		})
	}
}

func TestManifestDerivedPackageAssets(t *testing.T) {
	for _, application := range []string{"chatbot-mesh", "coding-agent", "agent-architecture"} {
		t.Run(application, func(t *testing.T) {
			manifest := loadRelease14Manifest(t, application)
			options := release14ManifestOptions(application)
			inventory, err := appmanifest.Resolve(manifest, options)
			if err != nil {
				t.Fatal(err)
			}
			roots := inventoryRootIDs(inventory)
			for _, asset := range manifest.UI.Assets {
				if !roots["ui-"+asset.ID] {
					t.Errorf("UI asset %s is absent from manifest-derived closure", asset.ID)
				}
			}
			for _, asset := range manifest.Package.Assets {
				if !roots["asset-"+asset.ID] {
					t.Errorf("package asset %s is absent from manifest-derived closure", asset.ID)
				}
			}

			mutated := manifest
			var removed string
			if len(mutated.Package.Assets) > 0 {
				removed = "asset-" + mutated.Package.Assets[0].ID
				mutated.Package.Assets = append([]appmanifest.PackageAsset(nil), mutated.Package.Assets[1:]...)
			} else {
				removed = "ui-" + mutated.UI.Assets[0].ID
				mutated.UI.Assets = append([]appmanifest.UIAsset(nil), mutated.UI.Assets[1:]...)
				if len(mutated.UI.Assets) == 0 {
					mutated.Capabilities["ui"] = appmanifest.Capability{Status: "not_applicable"}
				}
			}
			withoutAsset, err := appmanifest.Resolve(mutated, options)
			if err != nil {
				t.Fatal(err)
			}
			if inventoryRootIDs(withoutAsset)[removed] {
				t.Errorf("undeclared asset %s remained in package closure", removed)
			}
		})
	}
}

func TestApplicationPromotionAndUITokens(t *testing.T) {
	canonicalApplier := filepath.Join(release14CatalogRoot(), "agents", "applier")
	promoted := []string{
		"machine.yaml", "apply-machine.yaml", "rollout-machine.yaml",
		"tools.yaml", "apply-tools.yaml", "rollout-tools.yaml",
		"declarations.yaml", "apply-declarations.yaml",
	}
	canonicalSums := map[[sha256.Size]byte]string{}
	for _, name := range promoted {
		data := readRelease14File(t, filepath.Join(canonicalApplier, name))
		canonicalSums[sha256.Sum256(data)] = name
	}
	for _, application := range []string{"chatbot-mesh", "coding-agent", "agent-architecture"} {
		wrapper := filepath.Join(release14ApplicationRoot(application), "agents", "applier")
		var profile struct {
			Machine          string   `yaml:"machine"`
			Tools            []string `yaml:"tools"`
			ToolDeclarations []string `yaml:"tool_declarations"`
		}
		readRelease14YAML(t, filepath.Join(wrapper, "profile.yaml"), &profile)
		if profile.Machine != "../../catalog/applier/machine.yaml" ||
			!containsString(profile.Tools, "../../catalog/applier/tools.yaml") ||
			!containsString(profile.ToolDeclarations, "../../catalog/applier/declarations.yaml") {
			t.Errorf("%s applier does not compose the canonical implementation: %#v",
				application, profile)
		}
		err := filepath.WalkDir(filepath.Join(release14ApplicationRoot(application), "agents"),
			func(path string, entry fs.DirEntry, err error) error {
				if err != nil || entry.IsDir() || (filepath.Ext(path) != ".yaml" && filepath.Ext(path) != ".yml") {
					return err
				}
				sum := sha256.Sum256(readRelease14File(t, path))
				if copied := canonicalSums[sum]; copied != "" {
					t.Errorf("%s copies canonical applier asset %s", relativeToRepo(path), copied)
				}
				return nil
			})
		if err != nil {
			t.Fatal(err)
		}
	}

	canonicalTokens, err := filepath.Abs(filepath.Join(release14CatalogRoot(), "ui", "design-tokens.css"))
	if err != nil {
		t.Fatal(err)
	}
	for _, relative := range []string{
		"applications/chatbot-mesh/agents/chatbot/ui/app/src/App.css",
		"applications/chatbot-mesh/agents/observer/ui/src/App.css",
	} {
		path, err := filepath.Abs(filepath.Join("..", filepath.FromSlash(relative)))
		if err != nil {
			t.Fatal(err)
		}
		css := readRelease14File(t, path)
		if err := validateCanonicalTokenImport(canonicalTokens, path, css); err != nil {
			t.Error(err)
		}
		drifted := append(bytes.Clone(css), []byte("\n:root { --bg-primary: #000; }\n")...)
		if err := validateCanonicalTokenImport(canonicalTokens, path, drifted); err == nil {
			t.Errorf("%s token drift was accepted", relative)
		}
	}
}

func TestApplicationReadmeStatusAndRootClassification(t *testing.T) {
	requiredHeadings := []string{
		"Purpose",
		"Status",
		"Composition",
		"Capabilities",
		"Ownership Boundaries",
		"Run or Planned Entry Points",
		"Verification",
		"Documentation",
	}
	for _, application := range release14Applications {
		t.Run(application, func(t *testing.T) {
			manifest := loadRelease14Manifest(t, application)
			readme := string(readRelease14File(t,
				filepath.Join(release14ApplicationRoot(application), "README.md")))
			for _, heading := range requiredHeadings {
				if !strings.Contains(readme, "\n## "+heading+"\n") {
					t.Errorf("README missing heading %q", heading)
				}
			}
			if !strings.Contains(readme, "`"+manifest.ModuleStatus+"`") {
				t.Errorf("README does not name module_status %q", manifest.ModuleStatus)
			}
			architecture := string(readRelease14File(t,
				filepath.Join(release14ApplicationRoot(application), "docs", "ARCHITECTURE.yaml")))
			if !strings.Contains(architecture, "ownership: "+manifest.Ownership) {
				t.Errorf("architecture does not name manifest ownership %q", manifest.Ownership)
			}
			module := "applications/" + application
			switch manifest.ModuleStatus {
			case "implemented":
				if !contains(applicationModules, module) || contains(auditOnlyApplicationModules, module) {
					t.Errorf("implemented module has stale root classification")
				}
			case "audit_only":
				if contains(applicationModules, module) || !contains(auditOnlyApplicationModules, module) {
					t.Errorf("audit-only module has stale root classification")
				}
			}
		})
	}
}

func TestRelease14ApplicationMatrix(t *testing.T) {
	type expectation struct {
		ownership, moduleStatus  string
		localRoots, catalogRoots int
		agents, wrappers         int
	}
	want := map[string]expectation{
		"chatbot-mesh":       {"agent-owning", "implemented", 7, 2, 5, 2},
		"coding-agent":       {"composition-only", "implemented", 4, 5, 0, 4},
		"agent-architecture": {"composition-only", "implemented", 1, 3, 0, 1},
	}
	wrapperRoots := map[string]map[string]bool{
		"chatbot-mesh":       {"applier": true, "corpus-ingest": true},
		"coding-agent":       {"coding-planner-server": true, "coding-executor-server": true, "coding-critic-server": true, "applier": true},
		"agent-architecture": {"applier": true},
	}
	for _, application := range release14Applications {
		manifest := loadRelease14Manifest(t, application)
		expected := want[application]
		local, catalog, wrappers := 0, 0, 0
		for _, root := range manifest.Roots {
			switch root.Ownership {
			case "local":
				local++
				if wrapperRoots[application][root.ID] {
					wrappers++
				}
			case "catalog":
				catalog++
			}
		}
		agents := local - wrappers
		if manifest.Ownership != expected.ownership ||
			manifest.ModuleStatus != expected.moduleStatus ||
			local != expected.localRoots || catalog != expected.catalogRoots ||
			agents != expected.agents || wrappers != expected.wrappers {
			t.Errorf("%s matrix = ownership %s status %s local %d catalog %d agents %d wrappers %d; want %#v",
				application, manifest.Ownership, manifest.ModuleStatus,
				local, catalog, agents, wrappers, expected)
		}
	}
	var suite struct {
		Status    string `yaml:"status"`
		TestCases []struct {
			ID             string `yaml:"id"`
			Status         string `yaml:"status"`
			Command        string `yaml:"command"`
			PlannedCommand string `yaml:"planned_command"`
		} `yaml:"test_cases"`
	}
	readRelease14YAML(t, filepath.Join("..", "applications", "docs", "specs",
		"test-suites", "test-rel14.0-application-consistency.yaml"), &suite)
	if suite.Status != "implemented" {
		t.Errorf("Release 14 suite status = %q, want implemented", suite.Status)
	}
	wantSymbols := map[string]string{
		"TC2": "TestApplicationConsistencyManifests",
		"TC3": "TestApplicationActorGrammar",
		"TC4": "TestManifestDerivedPackageAssets",
		"TC5": "TestApplicationPromotionAndUITokens",
		"TC6": "TestApplicationReadmeStatusAndRootClassification",
		"TC7": "TestRelease14ApplicationMatrix",
	}
	for _, testCase := range suite.TestCases {
		symbol, applies := wantSymbols[testCase.ID]
		if !applies {
			continue
		}
		if testCase.Status != "implemented" || testCase.PlannedCommand != "" ||
			!strings.Contains(testCase.Command, symbol) {
			t.Errorf("%s is not backed by executable %s: %#v",
				testCase.ID, symbol, testCase)
		}
	}
}

func release14ReferenceResolverFixture(t *testing.T) (release14ReferenceResolver, string, string) {
	t.Helper()
	applicationRoot := filepath.Join(t.TempDir(), "fixture")
	catalogRoot := t.TempDir()
	profile := filepath.Join(applicationRoot, "agents", "local", "request-profile.yaml")
	machine := filepath.Join(applicationRoot, "agents", "local", "machine.yaml")
	catalogTarget := filepath.Join(catalogRoot, "agents", "library", "default.yaml")
	for filename, content := range map[string]string{
		profile:       "name: request\n",
		machine:       "name: machine\n",
		catalogTarget: "name: default\n",
	} {
		if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filename, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	inventory := appmanifest.Inventory{
		Roots: []appmanifest.RootProvenance{
			{
				Ownership: "local", Source: "application/agents/local/profile.yaml",
				RuntimePath: "applications/fixture/local/profile.yaml",
			},
			{
				Ownership: "catalog", Source: "catalog/agents/library/profile.yaml",
				RuntimePath: "agents/library/profile.yaml",
			},
		},
		Files: []appmanifest.InventoryFile{{
			Source:      "application/agents/local/request-profile.yaml",
			RuntimePath: "applications/fixture/local/request-profile.yaml",
		}, {
			Source:      "catalog/agents/library/default.yaml",
			RuntimePath: "agents/library/default.yaml",
		}},
	}
	return newRelease14ReferenceResolver(applicationRoot, catalogRoot, inventory),
		profile, catalogTarget
}

func loadRelease14Manifest(t *testing.T, application string) appmanifest.Manifest {
	t.Helper()
	manifest, err := appmanifest.Load(
		filepath.Join(release14ApplicationRoot(application), "agents", "application.yaml"),
		release14ManifestOptions(application),
	)
	if err != nil {
		t.Fatalf("load %s manifest: %v", application, err)
	}
	return manifest
}

func release14ManifestOptions(application string) appmanifest.Options {
	return appmanifest.Options{
		ApplicationRoot: release14ApplicationRoot(application),
		CatalogRoot:     release14CatalogRoot(),
	}
}

func release14ApplicationRoot(application string) string {
	return filepath.Join("..", "applications", application)
}

func release14CatalogRoot() string {
	return filepath.Join("..", "applications", "catalog")
}

func discoverPrimaryLocalProfiles(t *testing.T, applicationRoot string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(applicationRoot, "agents"))
	if err != nil {
		t.Fatal(err)
	}
	var profiles []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(applicationRoot, "agents", entry.Name(), "profile.yaml")
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			profiles = append(profiles, filepath.ToSlash(filepath.Join("agents", entry.Name(), "profile.yaml")))
		}
	}
	sort.Strings(profiles)
	return profiles
}

func validateRelease14Profile(t *testing.T, filename string, resolver release14ReferenceResolver) {
	t.Helper()
	var profile struct {
		Machine          string   `yaml:"machine"`
		Tools            []string `yaml:"tools"`
		ToolDeclarations []string `yaml:"tool_declarations"`
		RESTDefinitions  []string `yaml:"rest_definitions"`
	}
	readRelease14YAML(t, filename, &profile)
	if profile.Machine == "" {
		t.Errorf("%s has no machine", relativeToRepo(filename))
	} else if _, external, err := resolver.resolve(filename, profile.Machine); err != nil && !external {
		t.Error(err)
	}
	for _, reference := range profile.Tools {
		path, external, err := resolver.resolve(filename, reference)
		if err != nil {
			t.Error(err)
			continue
		}
		if external {
			continue
		}
		if !strings.HasSuffix(filepath.Base(path), "tools.yaml") {
			t.Errorf("%s tool selection %s does not use a tools name", relativeToRepo(filename), reference)
		}
		var selection struct {
			Tools []yaml.Node `yaml:"tools"`
		}
		readRelease14YAML(t, path, &selection)
		if len(selection.Tools) == 0 {
			t.Errorf("%s selects no tools", relativeToRepo(path))
		}
		for _, tool := range selection.Tools {
			if tool.Kind != yaml.ScalarNode {
				t.Errorf("%s contains a ToolDef; selections must be name-only", relativeToRepo(path))
			}
		}
	}
	for _, reference := range profile.ToolDeclarations {
		path, external, err := resolver.resolve(filename, reference)
		if err != nil {
			t.Error(err)
			continue
		}
		if !external && !strings.HasSuffix(filepath.Base(path), "declarations.yaml") {
			t.Errorf("%s ToolDef authority %s does not use a declarations name",
				relativeToRepo(filename), reference)
		}
	}
	for _, reference := range profile.RESTDefinitions {
		path, external, err := resolver.resolve(filename, reference)
		if err != nil {
			t.Error(err)
		} else if !external && !strings.HasSuffix(filepath.Base(path), "rest.yaml") {
			t.Errorf("%s REST definition %s does not use a rest name",
				relativeToRepo(filename), reference)
		}
	}
}

type release14ReferenceResolver struct {
	applicationRoot string
	catalogRoot     string
	sourceRuntimes  map[string][]string
	runtimeSources  map[string]string
}

func newRelease14ReferenceResolver(applicationRoot, catalogRoot string,
	inventory appmanifest.Inventory,
) release14ReferenceResolver {
	resolver := release14ReferenceResolver{
		applicationRoot: applicationRoot,
		catalogRoot:     catalogRoot,
		sourceRuntimes:  make(map[string][]string),
		runtimeSources:  make(map[string]string),
	}
	for _, file := range inventory.Files {
		resolver.sourceRuntimes[file.Source] = append(
			resolver.sourceRuntimes[file.Source], file.RuntimePath)
		resolver.runtimeSources[file.RuntimePath] = file.Source
	}
	for source := range resolver.sourceRuntimes {
		sort.Strings(resolver.sourceRuntimes[source])
	}
	return resolver
}

func (resolver release14ReferenceResolver) resolve(profile, reference string) (string, bool, error) {
	portable := filepath.ToSlash(strings.TrimSpace(reference))
	if portable == "" || strings.Contains(portable, `\`) {
		return "", false, fmt.Errorf("%s has empty or non-portable reference %s",
			relativeToRepo(profile), reference)
	}
	if strings.ContainsAny(portable, "*?[") {
		return "", false, fmt.Errorf("%s has unbounded glob reference %s",
			relativeToRepo(profile), reference)
	}
	cleanPortable := path.Clean(portable)
	if cleanPortable == "/opt/agent-core" || strings.HasPrefix(cleanPortable, "/opt/agent-core/") {
		return "", true, nil
	}
	windowsAbsolute := len(portable) >= 3 &&
		((portable[0] >= 'a' && portable[0] <= 'z') ||
			(portable[0] >= 'A' && portable[0] <= 'Z')) &&
		portable[1] == ':' && portable[2] == '/'
	if path.IsAbs(portable) || filepath.IsAbs(reference) || windowsAbsolute {
		return "", false, fmt.Errorf("%s has disallowed absolute reference %s",
			relativeToRepo(profile), reference)
	}
	profileSource, err := filepath.Rel(resolver.applicationRoot, profile)
	if err != nil || escapesRelease14Root(profileSource) {
		return "", false, fmt.Errorf("%s is outside its application root", relativeToRepo(profile))
	}
	profileSource = "application/" + filepath.ToSlash(profileSource)

	sourceRelative := filepath.Clean(filepath.Join(
		filepath.Dir(strings.TrimPrefix(profileSource, "application/")),
		filepath.FromSlash(portable),
	))
	sourceMissing := false
	if !strings.HasPrefix(portable, "agents/") && !escapesRelease14Root(sourceRelative) {
		if filename, exists, err := resolver.regularSourceFile("application/" + filepath.ToSlash(sourceRelative)); err != nil {
			return "", false, fmt.Errorf("%s has unsafe reference %s: %w",
				relativeToRepo(profile), reference, err)
		} else if exists {
			return filename, false, nil
		} else {
			sourceMissing = true
		}
	}

	runtimeTargets := make(map[string]bool)
	for _, profileRuntime := range resolver.sourceRuntimes[profileSource] {
		var runtime string
		if strings.HasPrefix(portable, "agents/") {
			runtime = path.Clean(portable)
		} else {
			runtime = path.Clean(path.Join(path.Dir(profileRuntime), portable))
		}
		if runtime == "." || runtime == ".." || strings.HasPrefix(runtime, "../") || path.IsAbs(runtime) {
			continue
		}
		runtimeTargets[runtime] = true
	}
	targets := make([]string, 0, len(runtimeTargets))
	for runtime := range runtimeTargets {
		targets = append(targets, runtime)
	}
	sort.Strings(targets)
	for _, runtime := range targets {
		source, declared := resolver.runtimeSources[runtime]
		if !declared {
			continue
		}
		filename, exists, err := resolver.regularSourceFile(source)
		if err != nil {
			return "", false, fmt.Errorf("%s has unsafe reference %s: %w",
				relativeToRepo(profile), reference, err)
		}
		if exists {
			return filename, strings.HasPrefix(source, "catalog/"), nil
		}
	}
	if sourceMissing {
		return "", false, fmt.Errorf("%s has dangling reference %s",
			relativeToRepo(profile), reference)
	}
	if len(runtimeTargets) > 0 {
		return "", false, fmt.Errorf("%s has reference %s outside declared runtime roots",
			relativeToRepo(profile), reference)
	}
	return "", false, fmt.Errorf("%s has dangling reference %s",
		relativeToRepo(profile), reference)
}

func (resolver release14ReferenceResolver) regularSourceFile(source string) (string, bool, error) {
	var root, relative string
	switch {
	case strings.HasPrefix(source, "application/"):
		root, relative = resolver.applicationRoot, strings.TrimPrefix(source, "application/")
	case strings.HasPrefix(source, "catalog/"):
		root, relative = resolver.catalogRoot, strings.TrimPrefix(source, "catalog/")
	default:
		return "", false, fmt.Errorf("source %s has no declared ownership", source)
	}
	if relative == "" || path.IsAbs(relative) || escapesRelease14Root(filepath.FromSlash(relative)) {
		return "", false, fmt.Errorf("source %s escapes its ownership root", source)
	}
	current := filepath.Clean(root)
	var info fs.FileInfo
	for _, component := range strings.Split(filepath.FromSlash(relative), string(filepath.Separator)) {
		current = filepath.Join(current, component)
		var err error
		info, err = os.Lstat(current)
		if os.IsNotExist(err) {
			return "", false, nil
		}
		if err != nil {
			return "", false, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", false, fmt.Errorf("source %s contains symlink %s", source, component)
		}
	}
	if !info.Mode().IsRegular() {
		return "", false, fmt.Errorf("source %s is not a regular file", source)
	}
	return current, true, nil
}

func escapesRelease14Root(relative string) bool {
	return filepath.IsAbs(relative) || relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func inventoryRootIDs(inventory appmanifest.Inventory) map[string]bool {
	result := make(map[string]bool, len(inventory.Roots))
	for _, root := range inventory.Roots {
		result[root.ID] = true
	}
	return result
}

func validateCanonicalTokenImport(canonical, path string, css []byte) error {
	first := strings.SplitN(string(css), "\n", 2)[0]
	const prefix = `@import "`
	if !strings.HasPrefix(first, prefix) || !strings.HasSuffix(first, `";`) {
		return fmt.Errorf("%s does not import canonical design tokens first", relativeToRepo(path))
	}
	imported := strings.TrimSuffix(strings.TrimPrefix(first, prefix), `";`)
	resolved := filepath.Clean(filepath.Join(filepath.Dir(path), filepath.FromSlash(imported)))
	if resolved != filepath.Clean(canonical) {
		return fmt.Errorf("%s token import resolves to %s, want %s",
			relativeToRepo(path), resolved, relativeToRepo(canonical))
	}
	if strings.Contains(string(css), "--bg-primary:") {
		return fmt.Errorf("%s contains copied canonical token values", relativeToRepo(path))
	}
	return nil
}

func readRelease14YAML(t *testing.T, path string, out any) {
	t.Helper()
	if err := yaml.Unmarshal(readRelease14File(t, path), out); err != nil {
		t.Fatalf("parse %s: %v", relativeToRepo(path), err)
	}
}

func readRelease14File(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", relativeToRepo(path), err)
	}
	return data
}

func relativeToRepo(path string) string {
	relative, err := filepath.Rel("..", path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(relative)
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
