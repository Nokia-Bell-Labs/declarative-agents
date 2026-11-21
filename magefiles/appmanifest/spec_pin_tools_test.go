// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package appmanifest

import (
	"strings"
	"testing"
)

// Pinning tests for application-dsl-spec 1.0 statements whose manifest rules
// no other test exercised (R-APP-002, R-APP-003, R-APP-004).

func TestSpecPinManifestModuleStatusAndRunnableModule(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Manifest)
		want string
	}{
		{"unknown module status", func(m *Manifest) { m.ModuleStatus = "shipping" }, "unknown module_status"},
		{"missing runnable module", func(m *Manifest) { delete(m.Capabilities, "runnable_module") }, "must declare runnable_module"},
		{"audit only claims runnable", func(m *Manifest) { m.ModuleStatus = "audit_only" }, "cannot claim runnable"},
		{"unknown capability status", func(m *Manifest) {
			m.Capabilities["packaged"] = Capability{Status: "done", Evidence: []string{"test"}}
		}, "unknown status"},
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

func TestSpecPinRootSourceNamesAProfileBelowAgents(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Manifest)
		want string
	}{
		{"source outside agents", func(m *Manifest) { m.Roots[1].Source = "profiles/local/profile.yaml" }, "below agents/<actor>"},
		{"source names no profile", func(m *Manifest) { m.Roots[1].Source = "agents/local/machine.yaml" }, "below agents/<actor>"},
		{"unknown root ownership", func(m *Manifest) { m.Roots[1].Ownership = "vendored" }, "unknown ownership"},
		{"root id not kebab", func(m *Manifest) { m.Roots[1].ID = "Local Root" }, "lower-kebab"},
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
