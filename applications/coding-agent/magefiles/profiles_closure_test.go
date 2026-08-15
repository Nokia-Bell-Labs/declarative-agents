// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestProfileClosureResolvesTransitiveRuntimeReferences(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "agents/planner/profile.yaml"), `name: planner
machine: machine.yaml
tools: [tools.yaml]
tool_declarations:
  - builtin.yaml
tool_config_dirs:
  - /opt/agent-core/tools/builtin/planner
`)
	writeTestFile(t, filepath.Join(root, "agents/planner/machine.yaml"), "name: planner\n")
	writeTestFile(t, filepath.Join(root, "agents/planner/tools.yaml"), "tools: [execute_task]\n")
	writeTestFile(t, filepath.Join(root, "agents/planner/builtin.yaml"), `tools:
  - name: execute_task
    config:
      profile: agents/executor/profile.yaml
`)
	writeTestFile(t, filepath.Join(root, "agents/executor/profile.yaml"), "name: executor\nmachine: machine.yaml\n")
	writeTestFile(t, filepath.Join(root, "agents/executor/machine.yaml"), "name: executor\n")

	closure := &profileClosure{sourceRoot: root, assets: map[string]string{}}
	if err := closure.enqueue("agents/planner/profile.yaml", "agents/planner/profile.yaml"); err != nil {
		t.Fatal(err)
	}
	if err := closure.resolve(); err != nil {
		t.Fatalf("resolve closure: %v", err)
	}
	got := sortedAssetDestinations(closure.assets)
	want := []string{
		"agents/executor/machine.yaml",
		"agents/executor/profile.yaml",
		"agents/planner/builtin.yaml",
		"agents/planner/machine.yaml",
		"agents/planner/profile.yaml",
		"agents/planner/tools.yaml",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("closure files = %#v, want %#v", got, want)
	}
}

func TestProfileClosureResolvesNestedRequestMachineAndBoundsProfileCycle(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "agents/collector/profile.yaml"), `name: collector
rest_definitions: [rest.yaml]
`)
	writeTestFile(t, filepath.Join(root, "agents/collector/rest.yaml"), `rest:
  servers:
    collector:
      endpoints:
        traces:
          binding: machine_request
          machine_request:
            profile: profile.yaml
            machine: query-machine.yaml
`)
	writeTestFile(t, filepath.Join(root, "agents/collector/query-machine.yaml"), "name: collector-query\n")

	closure := &profileClosure{sourceRoot: root, assets: map[string]string{}}
	if err := closure.enqueue("agents/collector/profile.yaml", "agents/collector/profile.yaml"); err != nil {
		t.Fatal(err)
	}
	if err := closure.resolve(); err != nil {
		t.Fatalf("resolve request-machine closure: %v", err)
	}
	got := sortedAssetDestinations(closure.assets)
	want := []string{
		"agents/collector/profile.yaml",
		"agents/collector/query-machine.yaml",
		"agents/collector/rest.yaml",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("request-machine closure files = %#v, want %#v", got, want)
	}
}

func TestProfileClosureRejectsDanglingReference(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "agents/planner/profile.yaml"), "name: planner\nmachine: missing.yaml\n")
	manifest := testProfileManifest("agents/planner/profile.yaml", "agents/planner/profile.yaml")
	_, err := assembleProfileClosure(manifest, root, filepath.Join(t.TempDir(), "profiles"), testPackageSource())
	if err == nil || !strings.Contains(err.Error(), "dangling profile reference agents/planner/missing.yaml") {
		t.Fatalf("assemble error = %v, want dangling reference", err)
	}
}

func TestProfileClosureRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "agents/planner/profile.yaml"), "name: planner\nmachine: ../../../outside.yaml\n")
	manifest := testProfileManifest("agents/planner/profile.yaml", "agents/planner/profile.yaml")
	_, err := assembleProfileClosure(manifest, root, filepath.Join(t.TempDir(), "profiles"), testPackageSource())
	if err == nil || !strings.Contains(err.Error(), "escapes catalog root") {
		t.Fatalf("assemble error = %v, want traversal rejection", err)
	}
}

func TestApplicationManifestRejectsTraversalAndAbsolutePaths(t *testing.T) {
	for _, tc := range []struct {
		name   string
		source string
	}{
		{name: "traversal", source: "../catalog/agents/planner/profile.yaml"},
		{name: "absolute", source: "/profiles/agents/planner/profile.yaml"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			appRoot := filepath.Join(t.TempDir(), "test")
			filename := filepath.Join(appRoot, "agents", "application.yaml")
			writeTestFile(t, filename, `schema_version: 1
application: test
ownership: agent-owning
module_status: implemented
capabilities:
  runnable_module: {status: implemented, evidence: [test]}
  packaged: {status: implemented, evidence: [test]}
roots:
  - id: planner
    ownership: catalog
    source: `+tc.source+`
    runtime_path: agents/planner/profile.yaml
    compatible_release: v0.20260724.0
runtime:
  mount_path: /profiles
  image_contains_profiles: false
`)
			if _, err := readApplicationProfileManifest(filename); err == nil {
				t.Fatalf("manifest source %q was accepted", tc.source)
			}
		})
	}
}

func TestApplicationManifestAcceptsRootCanonicalAndLegacyV0CompatibilityTags(t *testing.T) {
	for _, release := range []string{
		"v0.20260724.0",
		"applications/catalog/v0.20260724.0",
		"agent-profiles/v0.20260724.0",
	} {
		if !isCompatibleProfileRelease(release) {
			t.Errorf("compatible release %q was rejected", release)
		}
	}
	for _, release := range []string{
		"v1.0.0",
		"v0.",
		"v0.20260724.0/extra",
		"applications/catalog/v1.0.0",
		"applications/catalog/v0.",
		"applications/catalog/v0.20260724.0/extra",
	} {
		if isCompatibleProfileRelease(release) {
			t.Errorf("incompatible release %q was accepted", release)
		}
	}
}

func TestProfileClosureRejectsDisallowedAbsoluteReference(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "agents/planner/profile.yaml"), "name: planner\nmachine: /tmp/machine.yaml\n")
	manifest := testProfileManifest("agents/planner/profile.yaml", "agents/planner/profile.yaml")
	_, err := assembleProfileClosure(manifest, root, filepath.Join(t.TempDir(), "profiles"), testPackageSource())
	if err == nil || !strings.Contains(err.Error(), "absolute profile reference is not allowed") {
		t.Fatalf("assemble error = %v, want absolute path rejection", err)
	}
}

func TestProfileClosureRejectsConflictingDestinations(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "agents/a/profile.yaml"), "name: a\n")
	writeTestFile(t, filepath.Join(root, "agents/b/profile.yaml"), "name: b\n")
	manifest := testProfileManifest("agents/a/profile.yaml", "agents/shared/profile.yaml")
	manifest.Catalog.References = append(manifest.Catalog.References, profileReference{
		Role: "b", Source: "agents/b/profile.yaml", RuntimePath: "agents/shared/profile.yaml",
	})
	_, err := assembleProfileClosure(manifest, root, filepath.Join(t.TempDir(), "profiles"), testPackageSource())
	if err == nil || !strings.Contains(err.Error(), "conflicting destination") {
		t.Fatalf("assemble error = %v, want conflicting destination", err)
	}
}

func TestProfilePackageIsDeterministic(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "agents/planner/profile.yaml"), "name: planner\nmachine: machine.yaml\n")
	writeTestFile(t, filepath.Join(root, "agents/planner/machine.yaml"), "name: planner\n")
	manifest := testProfileManifest("agents/planner/profile.yaml", "agents/planner/profile.yaml")
	outputOne := filepath.Join(t.TempDir(), "profiles")
	outputTwo := filepath.Join(t.TempDir(), "profiles")
	if _, err := assembleProfileClosure(manifest, root, outputOne, testPackageSource()); err != nil {
		t.Fatal(err)
	}
	if _, err := assembleProfileClosure(manifest, root, outputTwo, testPackageSource()); err != nil {
		t.Fatal(err)
	}
	if one, two := snapshotTree(t, outputOne), snapshotTree(t, outputTwo); !reflect.DeepEqual(one, two) {
		t.Fatalf("package output differs:\nfirst: %#v\nsecond: %#v", one, two)
	}
}

func TestCheckoutFallbackDoesNotClaimReleaseTag(t *testing.T) {
	source, err := inspectPackageSource(t.TempDir(), "agent-profiles/v0.20260724.0")
	if err != nil {
		t.Fatal(err)
	}
	if source.Kind != "checkout" || source.Release != "" || source.Revision != "unversioned-checkout" {
		t.Fatalf("source = %#v, want an explicitly unversioned checkout", source)
	}
}

func TestPackageSourceRecordsExactCanonicalOrLegacyRelease(t *testing.T) {
	for _, release := range []string{
		"v0.20260724.0",
		"applications/catalog/v0.20260724.0",
		"agent-profiles/v0.20260724.0",
	} {
		t.Run(strings.ReplaceAll(release, "/", "-"), func(t *testing.T) {
			root := t.TempDir()
			writeTestFile(t, filepath.Join(root, "agents", "profile.yaml"), "name: fixture\n")
			for _, args := range [][]string{
				{"init"},
				{"add", "."},
				{"-c", "user.name=Coding Agent Test", "-c", "user.email=test@example.invalid", "commit", "-m", "fixture"},
				{"tag", release},
			} {
				if _, err := gitOutput(root, args...); err != nil {
					t.Fatalf("git %v: %v", args, err)
				}
			}

			source, err := inspectPackageSource(root, release)
			if err != nil {
				t.Fatal(err)
			}
			if source.Kind != "release" || source.Release != release ||
				source.CompatibleRelease != release || source.Dirty {
				t.Fatalf("source = %#v, want exact clean release %q", source, release)
			}

			writeTestFile(t, filepath.Join(root, "agents", "dirty.yaml"), "name: dirty\n")
			source, err = inspectPackageSource(root, release)
			if err != nil {
				t.Fatal(err)
			}
			if source.Kind != "checkout" || source.Release != "" || !source.Dirty {
				t.Fatalf("dirty source = %#v, want checkout provenance", source)
			}
		})
	}
}

func TestCodingApplicationManifestStagesEveryMountedProfile(t *testing.T) {
	appRoot := filepath.Clean("..")
	manifest, err := readApplicationProfileManifest(filepath.Join(appRoot, filepath.FromSlash(profileManifestPath)))
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Catalog.CompatibleRelease != "v0.20260804.0" {
		t.Fatalf("coding-agent compatible_release = %q, want applier-family release",
			manifest.Catalog.CompatibleRelease)
	}
	profilesRoot := filepath.Clean(filepath.Join(appRoot, "..", "catalog"))
	output := filepath.Join(t.TempDir(), "profiles")
	files, err := assembleProfileClosure(manifest, profilesRoot, output, testPackageSource())
	if err != nil {
		t.Fatalf("assemble canonical closure: %v", err)
	}
	staged := make(map[string]bool, len(files))
	for _, filename := range files {
		staged[filename] = true
	}
	wantRoles := map[string]string{
		"planner":          "agents/planner/profile.yaml",
		"executor":         "agents/executor/profile.yaml",
		"critic":           "agents/critic/profile.yaml",
		"critic-workspace": "agents/critic/profile-workspace.yaml",
		"collector":        "agents/collector/profile.yaml",
	}
	for _, ref := range manifest.Catalog.References {
		want, exists := wantRoles[ref.Role]
		if !exists {
			t.Errorf("unexpected application profile role %q", ref.Role)
			continue
		}
		delete(wantRoles, ref.Role)
		if ref.RuntimePath != want {
			t.Errorf("%s runtime path = %q, want %q", ref.Role, ref.RuntimePath, want)
		}
		if !staged[ref.RuntimePath] {
			t.Errorf("%s mounts %s but closure did not stage it", ref.Role, ref.RuntimePath)
		}
	}
	if len(wantRoles) != 0 {
		t.Errorf("application manifest is missing roles: %#v", wantRoles)
	}
	for _, required := range []string{
		"agents/planner/builtin.yaml",
		"agents/executor/llm/default.yaml",
		"agents/critic/point.yaml",
		"agents/critic/profile-workspace.yaml",
		"agents/critic/workspace-exec.yaml",
		"agents/collector/profile.yaml",
	} {
		if !staged[required] {
			t.Errorf("canonical closure missing transitive asset %s", required)
		}
	}
}

func TestCodingApplicationAgentsContainOnlyCompositionAndServingAssets(t *testing.T) {
	agentsDir := filepath.Join("..", "agents")
	var files []string
	err := filepath.WalkDir(agentsDir, func(filename string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			rel, err := filepath.Rel(agentsDir, filename)
			if err != nil {
				return err
			}
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"application.yaml",
		"applier/apply-profile.yaml",
		"applier/exec-declarations.yaml",
		"applier/profile.yaml",
		"applier/rest.yaml",
		"applier/rollout-profile.yaml",
		"critic/profile.yaml",
		"critic/rest.yaml",
		"executor/profile.yaml",
		"executor/rest.yaml",
		"planner/profile.yaml",
		"planner/request-declarations.yaml",
		"planner/request-machine.yaml",
		"planner/request-profile.yaml",
		"planner/request-tools.yaml",
		"planner/rest.yaml",
		"role-server/declarations.yaml",
		"role-server/machine.yaml",
		"role-server/tools.yaml",
	}
	if !reflect.DeepEqual(files, want) {
		t.Fatalf("application agents files = %#v, want composition-only %#v", files, want)
	}
}

func TestPackagedManifestRecordsCheckoutAndCompatibleRelease(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "agents/planner/profile.yaml"), "name: planner\n")
	manifest := testProfileManifest("agents/planner/profile.yaml", "agents/planner/profile.yaml")
	output := filepath.Join(t.TempDir(), "profiles")
	source := testPackageSource()
	if _, err := assembleProfileClosure(manifest, root, output, source); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(output, "package-manifest.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var packaged packagedProfileManifest
	if err := yaml.Unmarshal(data, &packaged); err != nil {
		t.Fatal(err)
	}
	if packaged.Source.Kind != "checkout" || packaged.Source.Release != "" {
		t.Fatalf("packaged source = %#v, falsely claims release", packaged.Source)
	}
	if packaged.Source.CompatibleRelease != "applications/catalog/v0.20260724.0" {
		t.Fatalf("compatible release = %q", packaged.Source.CompatibleRelease)
	}
}

func testProfileManifest(source, runtimePath string) applicationProfileManifest {
	var manifest applicationProfileManifest
	manifest.SchemaVersion = 1
	manifest.Application = "test"
	manifest.Catalog.CompatibleRelease = "applications/catalog/v0.20260724.0"
	manifest.Catalog.References = []profileReference{{
		Role: "planner", Source: source, RuntimePath: runtimePath,
	}}
	manifest.Runtime.MountPath = "/profiles"
	return manifest
}

func testPackageSource() packageSource {
	return packageSource{
		Kind:              "checkout",
		CompatibleRelease: "applications/catalog/v0.20260724.0",
		Revision:          "test-revision",
	}
}

func sortedAssetDestinations(assets map[string]string) []string {
	destinations := make([]string, 0, len(assets))
	for destination := range assets {
		destinations = append(destinations, destination)
	}
	sort.Strings(destinations)
	return destinations
}

func snapshotTree(t *testing.T, root string) map[string][]byte {
	t.Helper()
	snapshot := make(map[string][]byte)
	err := filepath.WalkDir(root, func(filename string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, filename)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(filename)
		if err != nil {
			return err
		}
		snapshot[filepath.ToSlash(rel)] = bytes.Clone(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}
