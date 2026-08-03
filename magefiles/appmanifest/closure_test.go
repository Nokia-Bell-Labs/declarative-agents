// Copyright (c) 2026 Nokia. All rights reserved.

package appmanifest

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestResolveCompleteClosureAndProvenance(t *testing.T) {
	appRoot, catalogRoot, manifest := completeClosureFixture(t)
	inventory, err := Resolve(manifest, Options{ApplicationRoot: appRoot, CatalogRoot: catalogRoot})
	if err != nil {
		t.Fatal(err)
	}
	got := inventoryRuntimePaths(inventory)
	want := []string{
		"agents/child/machine.yaml",
		"agents/child/profile.yaml",
		"agents/root/declarations.yaml",
		"agents/root/included.yaml",
		"agents/root/machine.yaml",
		"agents/root/openapi.yaml",
		"agents/root/point-declarations.yaml",
		"agents/root/point-machine.yaml",
		"agents/root/point-tools.yaml",
		"agents/root/profile.yaml",
		"agents/root/request-machine.yaml",
		"agents/root/request-profile.yaml",
		"agents/root/rest.yaml",
		"agents/root/tools.yaml",
		"applications/fixture-app/common/machine.yaml",
		"applications/fixture-app/local/profile.yaml",
		"applications/fixture-app/local/ui/app.js",
		"applications/fixture-app/local/ui/index.html",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("closure paths = %#v, want %#v", got, want)
	}
	for _, file := range inventory.Files {
		if !strings.HasPrefix(file.Checksum, "sha256:") || len(file.Checksum) != len("sha256:")+64 {
			t.Errorf("%s checksum = %q", file.RuntimePath, file.Checksum)
		}
		if len(file.Roots) == 0 || (!strings.HasPrefix(file.Source, "catalog/") &&
			!strings.HasPrefix(file.Source, "application/")) {
			t.Errorf("%s lacks provenance: %#v", file.RuntimePath, file)
		}
	}
	if gotRoots := inventoryRootIDs(inventory); !reflect.DeepEqual(gotRoots,
		[]string{"catalog-root", "local-root", "ui-local-ui"}) {
		t.Fatalf("root provenance = %v", gotRoots)
	}
}

func TestResolveRejectsDanglingEscapingSymlinkedAndCyclicReferences(t *testing.T) {
	tests := []struct {
		name    string
		profile string
		arrange func(*testing.T, string)
		want    string
	}{
		{"dangling", "name: root\nmachine: missing.yaml\n", nil, "dangling"},
		{"escaping", "name: root\nmachine: ../../../outside.yaml\n", nil, "escapes ownership root"},
		{"absolute", "name: root\nmachine: /tmp/machine.yaml\n", nil, "absolute reference"},
		{"glob", "name: root\ntool_config_dirs: ['configs/*.yaml']\n", nil, "unbounded glob"},
		{"symlinked", "name: root\nmachine: machine.yaml\n", func(t *testing.T, root string) {
			writeFixtureFile(t, filepath.Join(root, "real.yaml"), "name: machine\n")
			if err := os.Symlink(filepath.Join(root, "real.yaml"), filepath.Join(root, "agents/root/machine.yaml")); err != nil {
				t.Fatal(err)
			}
		}, "symlink"},
		{"cyclic", "name: root\nmachine: machine.yaml\n", func(t *testing.T, root string) {
			writeFixtureFile(t, filepath.Join(root, "agents/root/machine.yaml"), "profile: profile.yaml\n")
		}, "cyclic closure"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			appRoot, catalogRoot, manifest := minimalClosureFixture(t, test.profile)
			if test.arrange != nil {
				test.arrange(t, catalogRoot)
			}
			_, err := Resolve(manifest, Options{ApplicationRoot: appRoot, CatalogRoot: catalogRoot})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Resolve error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestResolveRejectsConflictingDestinationContent(t *testing.T) {
	appRoot, catalogRoot := newApplicationRoot(t), t.TempDir()
	writeFixtureFile(t, filepath.Join(catalogRoot, "agents/a/profile.yaml"), "name: a\nmachine: machine.yaml\n")
	writeFixtureFile(t, filepath.Join(catalogRoot, "agents/a/machine.yaml"), "name: machine-a\n")
	writeFixtureFile(t, filepath.Join(catalogRoot, "agents/b/profile.yaml"), "name: b\nmachine: machine.yaml\n")
	writeFixtureFile(t, filepath.Join(catalogRoot, "agents/b/machine.yaml"), "name: machine-b\n")
	manifest := closureManifest(
		Root{ID: "a", Ownership: "catalog", Source: "agents/a/profile.yaml",
			RuntimePath: "bundle/a-profile.yaml", CompatibleRelease: "v0.20260803.0"},
		Root{ID: "b", Ownership: "catalog", Source: "agents/b/profile.yaml",
			RuntimePath: "bundle/b-profile.yaml", CompatibleRelease: "v0.20260803.0"},
	)
	_, err := Resolve(manifest, Options{ApplicationRoot: appRoot, CatalogRoot: catalogRoot})
	if err == nil || !strings.Contains(err.Error(), "conflicting destination bundle/machine.yaml") {
		t.Fatalf("Resolve error = %v, want destination conflict", err)
	}
}

func TestResolveAllowsDuplicateDestinationWithIdenticalContent(t *testing.T) {
	appRoot, catalogRoot := newApplicationRoot(t), t.TempDir()
	for _, actor := range []string{"a", "b"} {
		writeFixtureFile(t, filepath.Join(catalogRoot, "agents", actor, "profile.yaml"),
			"name: "+actor+"\nmachine: machine.yaml\n")
		writeFixtureFile(t, filepath.Join(catalogRoot, "agents", actor, "machine.yaml"), "name: shared\n")
	}
	manifest := closureManifest(
		Root{ID: "a", Ownership: "catalog", Source: "agents/a/profile.yaml",
			RuntimePath: "bundle/a-profile.yaml", CompatibleRelease: "v0.20260803.0"},
		Root{ID: "b", Ownership: "catalog", Source: "agents/b/profile.yaml",
			RuntimePath: "bundle/b-profile.yaml", CompatibleRelease: "v0.20260803.0"},
	)
	inventory, err := Resolve(manifest, Options{ApplicationRoot: appRoot, CatalogRoot: catalogRoot})
	if err != nil {
		t.Fatal(err)
	}
	file := inventoryFile(inventory, "bundle/machine.yaml")
	if !reflect.DeepEqual(file.Roots, []string{"a", "b"}) ||
		file.Source != "catalog/agents/a/machine.yaml" {
		t.Fatalf("merged duplicate = %#v", file)
	}
}

func TestResolveIsByteDeterministic(t *testing.T) {
	appRoot, catalogRoot, manifest := completeClosureFixture(t)
	var previous []byte
	for iteration := 0; iteration < 10; iteration++ {
		inventory, err := Resolve(manifest, Options{ApplicationRoot: appRoot, CatalogRoot: catalogRoot})
		if err != nil {
			t.Fatal(err)
		}
		data, err := yaml.Marshal(inventory)
		if err != nil {
			t.Fatal(err)
		}
		if iteration > 0 && !bytes.Equal(data, previous) {
			t.Fatalf("inventory differs on iteration %d:\n%s\n%s", iteration, previous, data)
		}
		previous = data
	}
}

func TestResolveBoundsClosureGrowth(t *testing.T) {
	appRoot, catalogRoot, manifest := minimalClosureFixture(t, "name: root\nmachine: machine.yaml\n")
	writeFixtureFile(t, filepath.Join(catalogRoot, "agents/root/machine.yaml"), "name: machine\n")
	if _, err := Resolve(manifest, Options{
		ApplicationRoot: appRoot, CatalogRoot: catalogRoot, MaxFiles: 1,
	}); err == nil || !strings.Contains(err.Error(), "maximum of 1 files") {
		t.Fatalf("bounded closure error = %v", err)
	}
}

func completeClosureFixture(t *testing.T) (string, string, Manifest) {
	t.Helper()
	appRoot, catalogRoot := newApplicationRoot(t), t.TempDir()
	writeFixtureFile(t, filepath.Join(catalogRoot, "agents/root/profile.yaml"), `name: root
machine: machine.yaml
tools: [tools.yaml]
tool_declarations: [declarations.yaml]
tool_config_dirs: [/opt/agent-core/tools/builtin]
rest_definitions: [rest.yaml]
`)
	for name, content := range map[string]string{
		"machine.yaml":            "name: root-machine\n",
		"tools.yaml":              "tools: [child]\n",
		"included.yaml":           "tools: []\n",
		"point-machine.yaml":      "name: point\n",
		"point-tools.yaml":        "tools: [point]\n",
		"point-declarations.yaml": "tools: []\n",
		"request-profile.yaml":    "name: request\nmachine: request-machine.yaml\ntools: [tools.yaml]\n",
		"request-machine.yaml":    "name: request-machine\n",
		"openapi.yaml":            "openapi: 3.1.0\ninfo: {title: fixture, version: v1}\npaths: {}\n",
	} {
		writeFixtureFile(t, filepath.Join(catalogRoot, "agents/root", name), content)
	}
	writeFixtureFile(t, filepath.Join(catalogRoot, "agents/root/declarations.yaml"), `includes: [included.yaml]
tools:
  - name: child
    config:
      profile: agents/child/profile.yaml
      point_machine: point-machine.yaml
      point_tools: point-tools.yaml
      point_tool_declarations: [point-declarations.yaml, /opt/agent-core/tools/exec/all.yaml]
`)
	writeFixtureFile(t, filepath.Join(catalogRoot, "agents/root/rest.yaml"), `rest:
  openapi:
    fixture: {path: openapi.yaml}
  servers:
    fixture:
      endpoints:
        run:
          binding: machine_request
          machine_request: {profile: request-profile.yaml, machine: request-machine.yaml}
`)
	writeFixtureFile(t, filepath.Join(catalogRoot, "agents/child/profile.yaml"),
		"name: child\nmachine: machine.yaml\ntools: [../root/tools.yaml]\n")
	writeFixtureFile(t, filepath.Join(catalogRoot, "agents/child/machine.yaml"), "name: child-machine\n")
	writeFixtureFile(t, filepath.Join(appRoot, "agents/local/profile.yaml"),
		"name: local\nmachine: ../common/machine.yaml\ntools: [agents/root/tools.yaml]\n")
	writeFixtureFile(t, filepath.Join(appRoot, "agents/common/machine.yaml"), "name: local-common\n")
	writeFixtureFile(t, filepath.Join(appRoot, "agents/local/ui/index.html"), "<html></html>\n")
	writeFixtureFile(t, filepath.Join(appRoot, "agents/local/ui/app.js"), "console.log('fixture')\n")
	writeFixtureFile(t, filepath.Join(appRoot, "agents/local/rest.yaml"), `rest:
  servers:
    local:
      endpoints:
        ui:
          binding: static_assets
          static_assets: {root: "${LOCAL_UI_ROOT:-agents/local/ui}", index: index.html}
`)
	manifest := closureManifest(
		Root{ID: "catalog-root", Ownership: "catalog", Source: "agents/root/profile.yaml",
			RuntimePath: "agents/root/profile.yaml", CompatibleRelease: "v0.20260803.0"},
		Root{ID: "local-root", Ownership: "local", Source: "agents/local/profile.yaml",
			RuntimePath: "applications/fixture-app/local/profile.yaml"},
	)
	manifest.Capabilities["ui"] = Capability{Status: "implemented", Evidence: []string{"UI test"}}
	manifest.UI.Assets = []UIAsset{{
		ID: "local-ui", Owner: "local-root", Source: "agents/local/ui",
		RuntimePath:    "applications/fixture-app/local/ui",
		RESTDefinition: "agents/local/rest.yaml", SharedTokens: "canonical",
	}}
	return appRoot, catalogRoot, manifest
}

func minimalClosureFixture(t *testing.T, profile string) (string, string, Manifest) {
	t.Helper()
	appRoot, catalogRoot := newApplicationRoot(t), t.TempDir()
	writeFixtureFile(t, filepath.Join(catalogRoot, "agents/root/profile.yaml"), profile)
	return appRoot, catalogRoot, closureManifest(Root{
		ID: "root", Ownership: "catalog", Source: "agents/root/profile.yaml",
		RuntimePath: "agents/root/profile.yaml", CompatibleRelease: "v0.20260803.0",
	})
}

func closureManifest(roots ...Root) Manifest {
	return Manifest{
		SchemaVersion: SchemaVersion, Application: "fixture-app", Ownership: "fixture-team",
		ModuleStatus: "implemented",
		Capabilities: map[string]Capability{
			"runnable_module": {Status: "implemented", Evidence: []string{"test"}},
			"packaged":        {Status: "implemented", Evidence: []string{"test"}},
		},
		Roots: roots, Runtime: Runtime{MountPath: "/profiles"},
	}
}

func inventoryRuntimePaths(inventory Inventory) []string {
	paths := make([]string, len(inventory.Files))
	for index, file := range inventory.Files {
		paths[index] = file.RuntimePath
	}
	return paths
}

func inventoryRootIDs(inventory Inventory) []string {
	ids := make([]string, len(inventory.Roots))
	for index, root := range inventory.Roots {
		ids[index] = root.ID
	}
	return ids
}

func inventoryFile(inventory Inventory, runtimePath string) InventoryFile {
	for _, file := range inventory.Files {
		if file.RuntimePath == runtimePath {
			return file
		}
	}
	return InventoryFile{}
}
