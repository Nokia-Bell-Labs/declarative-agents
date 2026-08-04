// Copyright (c) 2026 Nokia. All rights reserved.

package appmanifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestLoadValidManifest(t *testing.T) {
	appRoot, catalogRoot := manifestFixture(t)
	filename := writeManifest(t, appRoot, validManifest())
	manifest, err := Load(filename, Options{ApplicationRoot: appRoot, CatalogRoot: catalogRoot})
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Application != "fixture-app" || len(manifest.Roots) != 2 ||
		manifest.Roots[0].RuntimePath != "agents/catalog/profile.yaml" {
		t.Fatalf("loaded manifest = %#v", manifest)
	}
}

func TestLoadRejectsInvalidSchemaPathsDuplicatesAndCapabilities(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Manifest)
		want string
	}{
		{"schema", func(m *Manifest) { m.SchemaVersion = 99 }, "unsupported"},
		{"application identity", func(m *Manifest) { m.Application = "Bad Name" }, "lower-kebab"},
		{"application root identity", func(m *Manifest) { m.Application = "other-app" }, "does not match"},
		{"missing ownership", func(m *Manifest) { m.Ownership = "" }, "unknown application ownership"},
		{"unknown ownership", func(m *Manifest) { m.Ownership = "composition" }, "unknown application ownership"},
		{"absolute source", func(m *Manifest) { m.Roots[0].Source = "/tmp/profile.yaml" }, "portable relative"},
		{"source traversal", func(m *Manifest) { m.Roots[0].Source = "agents/../profile.yaml" }, "traversal"},
		{"duplicate root", func(m *Manifest) { m.Roots[1].ID = m.Roots[0].ID }, "duplicate root"},
		{"normalized runtime duplicate", func(m *Manifest) {
			m.Roots[1].RuntimePath = "agents/catalog/./profile.yaml"
		}, "duplicate normalized runtime"},
		{"catalog compatibility missing", func(m *Manifest) {
			m.Roots[0].CompatibleRelease = ""
		}, "compatible_release"},
		{"local compatibility forbidden", func(m *Manifest) {
			m.Roots[1].CompatibleRelease = "v0.20260803.0"
		}, "must not declare"},
		{"unknown capability", func(m *Manifest) {
			m.Capabilities["telepathy"] = Capability{Status: "implemented", Evidence: []string{"test"}}
		}, "unknown capability"},
		{"capability evidence", func(m *Manifest) {
			m.Capabilities["packaged"] = Capability{Status: "implemented"}
		}, "requires evidence"},
		{"helm requires package", func(m *Manifest) {
			m.Capabilities["helm_managed"] = Capability{Status: "implemented", Evidence: []string{"chart"}}
			m.Capabilities["packaged"] = Capability{Status: "not_applicable"}
		}, "requires packaged"},
		{"deployment root", func(m *Manifest) {
			m.Deployment.Entries[0].Root = "missing"
		}, "undeclared root"},
		{"UI capability", func(m *Manifest) {
			m.Capabilities["ui"] = Capability{Status: "not_applicable"}
		}, "applicable ui"},
		{"UI runtime duplicate", func(m *Manifest) {
			m.UI.Assets[0].RuntimePath = m.Roots[1].RuntimePath
		}, "duplicate normalized runtime"},
		{"UI ownership mismatch", func(m *Manifest) {
			m.UI.Assets[0].Ownership = "catalog"
		}, "does not match owner"},
		{"UI package traversal", func(m *Manifest) {
			m.UI.Assets[0].PackagePath = "../ui"
		}, "traversal"},
		{"package ownership mismatch", func(m *Manifest) {
			m.Package.Assets[0].Ownership = "local"
		}, "does not match owner"},
		{"package destination duplicate", func(m *Manifest) {
			m.Package.Assets[0].PackagePath = m.UI.Assets[0].PackagePath
		}, "duplicate normalized package"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			appRoot, catalogRoot := manifestFixture(t)
			manifest := validManifest()
			test.edit(&manifest)
			_, err := Load(writeManifest(t, appRoot, manifest),
				Options{ApplicationRoot: appRoot, CatalogRoot: catalogRoot})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Load error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestLoadRejectsUnknownFieldsAndSymlinkedSources(t *testing.T) {
	t.Run("unknown field", func(t *testing.T) {
		appRoot, catalogRoot := manifestFixture(t)
		filename := writeManifest(t, appRoot, validManifest())
		data, err := os.ReadFile(filename)
		if err != nil {
			t.Fatal(err)
		}
		data = append(data, []byte("invented_field: true\n")...)
		if err := os.WriteFile(filename, data, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(filename, Options{ApplicationRoot: appRoot, CatalogRoot: catalogRoot}); err == nil ||
			!strings.Contains(err.Error(), "field invented_field") {
			t.Fatalf("unknown-field error = %v", err)
		}
	})
	t.Run("symlink", func(t *testing.T) {
		appRoot, catalogRoot := manifestFixture(t)
		target := filepath.Join(catalogRoot, "agents", "catalog", "profile.yaml")
		link := filepath.Join(catalogRoot, "agents", "catalog", "linked-profile.yaml")
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
		manifest := validManifest()
		manifest.Roots[0].Source = "agents/catalog/linked-profile.yaml"
		if _, err := Load(writeManifest(t, appRoot, manifest),
			Options{ApplicationRoot: appRoot, CatalogRoot: catalogRoot}); err == nil ||
			!strings.Contains(err.Error(), "symlink") {
			t.Fatalf("symlink error = %v", err)
		}
	})
}

func TestLoadValidatesTemplatedRESTUIBinding(t *testing.T) {
	appRoot, catalogRoot := manifestFixture(t)
	writeFixtureFile(t, filepath.Join(appRoot, "agents/local/rest.yaml"), `rest:
  limits:
    query:
      network:
        ports: [${QUERY_PORT:-18193}]
  servers:
    local:
      address: ${BIND_HOST:-127.0.0.1}:${QUERY_PORT:-18193}
      endpoints:
        ui:
          binding: static_assets
          static_assets: {root: agents/local/ui, index: index.html}
`)
	if _, err := Load(writeManifest(t, appRoot, validManifest()),
		Options{ApplicationRoot: appRoot, CatalogRoot: catalogRoot}); err != nil {
		t.Fatalf("templated REST definition was rejected: %v", err)
	}
}

func TestLoadAllowsPlannedMissingRootOnlyForAuditOnlyModule(t *testing.T) {
	appRoot, catalogRoot := manifestFixture(t)
	manifest := validManifest()
	manifest.ModuleStatus = "audit_only"
	manifest.Capabilities["runnable_module"] = Capability{Status: "audit_only"}
	manifest.Capabilities["packaged"] = Capability{Status: "not_applicable"}
	manifest.Capabilities["ui"] = Capability{Status: "not_applicable"}
	manifest.UI.Assets = nil
	manifest.Package.Assets = nil
	manifest.Deployment.Entries = nil
	manifest.Roots = []Root{{
		ID: "future", Ownership: "local", Source: "agents/future/profile.yaml",
		RuntimePath: "agents/future/profile.yaml", Planned: true,
	}}
	if _, err := Load(writeManifest(t, appRoot, manifest),
		Options{ApplicationRoot: appRoot, CatalogRoot: catalogRoot}); err != nil {
		t.Fatal(err)
	}
}

func validManifest() Manifest {
	return Manifest{
		SchemaVersion: SchemaVersion,
		Application:   "fixture-app",
		Ownership:     "agent-owning",
		ModuleStatus:  "implemented",
		Capabilities: map[string]Capability{
			"runnable_module": {Status: "implemented", Evidence: []string{"mage test"}},
			"packaged":        {Status: "implemented", Evidence: []string{"package test"}},
			"ui":              {Status: "implemented", Evidence: []string{"UI test"}},
		},
		Roots: []Root{
			{ID: "catalog-root", Ownership: "catalog", Source: "agents/catalog/profile.yaml",
				RuntimePath: "agents/catalog/profile.yaml", CompatibleRelease: "applications/catalog/v0.20260803.0"},
			{ID: "local-root", Ownership: "local", Source: "agents/local/profile.yaml",
				RuntimePath: "applications/fixture-app/local/profile.yaml"},
		},
		Runtime: Runtime{MountPath: "/profiles"},
		Deployment: Deployment{Entries: []DeploymentEntry{{
			ID: "local-server", Root: "local-root", Workload: "local",
			ProfilePath: "applications/fixture-app/local/profile.yaml", MountPath: "/profiles",
		}}},
		UI: UI{Assets: []UIAsset{{
			ID: "local-ui", Owner: "local-root", Ownership: "local", Source: "agents/local/ui",
			RuntimePath:    "applications/fixture-app/local/ui",
			PackagePath:    "applications/fixture-app/local/ui",
			RESTDefinition: "agents/local/rest.yaml", SharedTokens: "canonical",
		}}},
		Package: Package{Assets: []PackageAsset{{
			ID: "catalog-docs", Owner: "catalog-root", Ownership: "catalog",
			Source: "docs", RuntimePath: "docs", PackagePath: "curator/docs",
		}}},
	}
}

func manifestFixture(t *testing.T) (string, string) {
	t.Helper()
	appRoot, catalogRoot := newApplicationRoot(t), t.TempDir()
	writeFixtureFile(t, filepath.Join(catalogRoot, "agents/catalog/profile.yaml"), "name: catalog\n")
	writeFixtureFile(t, filepath.Join(catalogRoot, "docs/index.yaml"), "title: fixture\n")
	writeFixtureFile(t, filepath.Join(appRoot, "agents/local/profile.yaml"), "name: local\n")
	writeFixtureFile(t, filepath.Join(appRoot, "agents/local/ui/index.html"), "<html></html>\n")
	writeFixtureFile(t, filepath.Join(appRoot, "agents/local/rest.yaml"), `rest:
  servers:
    local:
      endpoints:
        ui:
          binding: static_assets
          static_assets: {root: agents/local/ui, index: index.html}
`)
	return appRoot, catalogRoot
}

func newApplicationRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "fixture-app")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func writeManifest(t *testing.T, appRoot string, manifest Manifest) string {
	t.Helper()
	data, err := yaml.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(appRoot, "agents", "application.yaml")
	writeFixtureFile(t, filename, string(data))
	return filename
}

func writeFixtureFile(t *testing.T, filename, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
