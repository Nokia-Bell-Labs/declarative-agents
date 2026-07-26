// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestDeploymentPackageContainsExactRoleClosures(t *testing.T) {
	root, manifest, cleanup := packageCanonicalDeployment(t)
	defer cleanup()
	want := map[string][]string{
		"planner": {
			"agents/planner/llm/default.yaml",
			"applications/coding-agent/common/declarations.yaml",
			"applications/coding-agent/common/machine.yaml",
			"applications/coding-agent/common/tools.yaml",
			"applications/coding-agent/planner/profile.yaml",
			"applications/coding-agent/planner/request-declarations.yaml",
			"applications/coding-agent/planner/request-machine.yaml",
			"applications/coding-agent/planner/request-profile.yaml",
			"applications/coding-agent/planner/request-tools.yaml",
			"applications/coding-agent/planner/rest.yaml",
		},
		"executor": {
			"agents/executor/llm/default.yaml",
			"agents/executor/machine.yaml",
			"agents/executor/profile.yaml",
			"agents/executor/tools.yaml",
			"applications/coding-agent/common/declarations.yaml",
			"applications/coding-agent/common/machine.yaml",
			"applications/coding-agent/common/tools.yaml",
			"applications/coding-agent/executor/profile.yaml",
			"applications/coding-agent/executor/rest.yaml",
		},
		"critic": {
			"agents/critic/machine-workspace.yaml",
			"agents/critic/profile-workspace.yaml",
			"agents/critic/tools-workspace.yaml",
			"agents/critic/workspace-exec.yaml",
			"applications/coding-agent/common/declarations.yaml",
			"applications/coding-agent/common/machine.yaml",
			"applications/coding-agent/common/tools.yaml",
			"applications/coding-agent/critic/profile.yaml",
			"applications/coding-agent/critic/rest.yaml",
		},
	}
	if len(manifest.Shards) != len(want) {
		t.Fatalf("shards = %d, want %d", len(manifest.Shards), len(want))
	}
	for role, expected := range want {
		roleManifest := readRolePackageManifest(t, filepath.Join(root, "manifests", role+".yaml"))
		if !reflect.DeepEqual(roleManifest.Files, expected) {
			t.Errorf("%s files:\n got %#v\nwant %#v", role, roleManifest.Files, expected)
		}
		if roleManifest.Profile != "applications/coding-agent/"+role+"/profile.yaml" {
			t.Errorf("%s profile = %q", role, roleManifest.Profile)
		}
		if len(roleManifest.ConfigMaps) != 1 ||
			!reflect.DeepEqual(roleManifest.ConfigMaps[0].Files, expected) {
			t.Errorf("%s ConfigMap partition does not cover exact closure: %#v", role, roleManifest.ConfigMaps)
		}
		for _, other := range servingRoles {
			if other == role {
				continue
			}
			for _, filename := range roleManifest.Files {
				if strings.HasPrefix(filename, "applications/coding-agent/"+other+"/") {
					t.Errorf("%s shard contains %s application asset %s", role, other, filename)
				}
			}
		}
	}
}

func TestDeploymentPackageIsDeterministic(t *testing.T) {
	roots, manifest := canonicalDeploymentInputs(t)
	first := filepath.Join(t.TempDir(), "profiles")
	second := filepath.Join(t.TempDir(), "profiles")
	source := testPackageSource()
	if _, err := packageServingDeployment(roots.Application, roots.Profiles, first, manifest, source); err != nil {
		t.Fatal(err)
	}
	if _, err := packageServingDeployment(roots.Application, roots.Profiles, second, manifest, source); err != nil {
		t.Fatal(err)
	}
	if one, two := snapshotTree(t, first), snapshotTree(t, second); !reflect.DeepEqual(one, two) {
		t.Fatal("role package output is not reproducible")
	}
}

func TestConfigMapPartitionsAreDeterministicAtBoundary(t *testing.T) {
	root := t.TempDir()
	assets := map[string]string{
		"a.yaml": "source/a.yaml",
		"b.yaml": "source/b.yaml",
		"c.yaml": "source/c.yaml",
	}
	writeTestFile(t, filepath.Join(root, "source/a.yaml"), "1234")
	writeTestFile(t, filepath.Join(root, "source/b.yaml"), "5678")
	writeTestFile(t, filepath.Join(root, "source/c.yaml"), "90")
	files := []string{"a.yaml", "b.yaml", "c.yaml"}
	partitions, err := partitionConfigMapFiles(root, assets, files, 18)
	if err != nil {
		t.Fatal(err)
	}
	want := []configMapPartition{
		{Index: 0, SizeBytes: 10, Files: []string{"a.yaml"}},
		{Index: 1, SizeBytes: 18, Files: []string{"b.yaml", "c.yaml"}},
	}
	if !reflect.DeepEqual(partitions, want) {
		t.Fatalf("partitions = %#v, want %#v", partitions, want)
	}
}

func TestConfigMapPartitionRejectsOversizedEntry(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "large.yaml"), strings.Repeat("x", 32))
	_, err := partitionConfigMapFiles(
		root,
		map[string]string{"large.yaml": "large.yaml"},
		[]string{"large.yaml"},
		16,
	)
	if err == nil || !strings.Contains(err.Error(), "single entry cannot be sharded") {
		t.Fatalf("oversized entry error = %v", err)
	}
}

func TestConfigMapPartitionRejectsEncodedKeyConflict(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "one.yaml"), "one")
	writeTestFile(t, filepath.Join(root, "two.yaml"), "two")
	_, err := partitionConfigMapFiles(
		root,
		map[string]string{"a/b.yaml": "one.yaml", "a__b.yaml": "two.yaml"},
		[]string{"a/b.yaml", "a__b.yaml"},
		128,
	)
	if err == nil || !strings.Contains(err.Error(), "ConfigMap key conflict") {
		t.Fatalf("key conflict error = %v", err)
	}
}

func TestDeploymentSourceRejectsSymlink(t *testing.T) {
	app := t.TempDir()
	profiles := t.TempDir()
	writeTestFile(t, filepath.Join(app, "agents/serving/planner/profile.yaml"), "name: planner\n")
	writeTestFile(t, filepath.Join(profiles, "outside.yaml"), "name: outside\n")
	if err := os.MkdirAll(filepath.Join(profiles, "agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(profiles, "outside.yaml"), filepath.Join(profiles, "agents/link.yaml")); err != nil {
		t.Fatal(err)
	}
	if _, cleanup, err := stageDeploymentSource(app, profiles); err == nil {
		cleanup()
		t.Fatal("deployment source accepted a symlink")
	}
}

func TestDeploymentServingReferenceRejectsDanglingAsset(t *testing.T) {
	root := t.TempDir()
	ref := profileReference{
		Role: "planner", RuntimePath: "applications/coding-agent/planner/profile.yaml",
	}
	if _, err := resolveServingRoleClosure(root, ref); err == nil ||
		!strings.Contains(err.Error(), "dangling profile reference") {
		t.Fatalf("dangling serving reference error = %v", err)
	}
}

func TestDeploymentServingReferencesRequireCanonicalApplicationPaths(t *testing.T) {
	references := []profileReference{
		{Role: "planner", Source: "agents/copied-planner/profile.yaml", RuntimePath: "applications/coding-agent/planner/profile.yaml"},
		{Role: "executor", Source: "agents/serving/executor/profile.yaml", RuntimePath: "applications/coding-agent/executor/profile.yaml"},
		{Role: "critic", Source: "agents/serving/critic/profile.yaml", RuntimePath: "applications/coding-agent/critic/profile.yaml"},
	}
	if err := validateServingReferences(references); err == nil ||
		!strings.Contains(err.Error(), "source") {
		t.Fatalf("non-canonical serving source error = %v", err)
	}
}

func TestDeploymentManifestPreservesProfileFreeRuntime(t *testing.T) {
	_, manifest, cleanup := packageCanonicalDeployment(t)
	defer cleanup()
	if manifest.ImageContainsProfiles {
		t.Fatal("deployment package claims profiles are baked into the image")
	}
	if manifest.MountPath != "/profiles" || manifest.ConfigMapPayloadLimit != configMapPayloadLimit {
		t.Fatalf("deployment contract = %#v", manifest)
	}
	if manifest.ApplicationSource.Revision == "" {
		t.Fatal("deployment package omits application source revision")
	}
}

func packageCanonicalDeployment(t *testing.T) (string, deploymentPackageManifest, func()) {
	t.Helper()
	roots, manifest := canonicalDeploymentInputs(t)
	output := filepath.Join(t.TempDir(), "profiles")
	if _, err := packageServingDeployment(
		roots.Application, roots.Profiles, output, manifest, testPackageSource(),
	); err != nil {
		t.Fatal(err)
	}
	var packaged deploymentPackageManifest
	readYAMLFile(t, filepath.Join(output, "deployment-manifest.yaml"), &packaged)
	return output, packaged, func() {}
}

func canonicalDeploymentInputs(t *testing.T) (integrationRoots, applicationProfileManifest) {
	t.Helper()
	app := filepath.Clean("..")
	roots := integrationRoots{
		Application: app,
		Core:        filepath.Clean(filepath.Join(app, "..", "..", "agent-core")),
		Profiles:    filepath.Clean(filepath.Join(app, "..", "catalog")),
	}
	manifest, err := readApplicationProfileManifest(filepath.Join(app, filepath.FromSlash(profileManifestPath)))
	if err != nil {
		t.Fatal(err)
	}
	return roots, manifest
}

func readRolePackageManifest(t *testing.T, filename string) rolePackageManifest {
	t.Helper()
	var manifest rolePackageManifest
	readYAMLFile(t, filename, &manifest)
	return manifest
}

func readYAMLFile(t *testing.T, filename string, value any) {
	t.Helper()
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(data, value); err != nil {
		t.Fatal(err)
	}
}
