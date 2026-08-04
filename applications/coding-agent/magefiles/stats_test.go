// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCollectCodingApplicationStatsReportsCompositionWithoutAgents(t *testing.T) {
	manifest := `roots:
  - {id: planner, ownership: catalog}
  - {id: executor, ownership: catalog}
  - {id: critic, ownership: catalog}
  - {id: critic-workspace, ownership: catalog}
  - {id: collector, ownership: catalog}
  - {id: coding-planner-server, ownership: local}
  - {id: applier, ownership: local}
runtime:
  image_contains_profiles: false
deployment:
  entries:
    - id: planner
    - id: executor
    - id: critic
    - id: applier
    - id: collector
ui:
  assets:
    - id: collector
`
	path := filepath.Join(t.TempDir(), "application.yaml")
	if err := os.WriteFile(path, []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}

	stats, err := collectCodingApplicationStats(path)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Application.Ownership != "composition" {
		t.Errorf("ownership = %q, want composition", stats.Application.Ownership)
	}
	if stats.Application.AgentsContributed != 0 {
		t.Errorf("agents_contributed = %d, want 0", stats.Application.AgentsContributed)
	}
	if stats.Application.CanonicalReferences != 5 ||
		!reflect.DeepEqual(stats.Application.CanonicalRoles,
			[]string{"planner", "executor", "critic", "critic-workspace", "collector"}) {
		t.Errorf("canonical references = %d %#v",
			stats.Application.CanonicalReferences, stats.Application.CanonicalRoles)
	}
	if stats.Application.ServingProfiles != 5 ||
		!reflect.DeepEqual(stats.Application.ServingRoles,
			[]string{"planner", "executor", "critic", "applier", "collector"}) {
		t.Errorf("serving profiles = %d %#v",
			stats.Application.ServingProfiles, stats.Application.ServingRoles)
	}
	if !stats.Application.ProfileFreeRuntime {
		t.Error("profile_free_runtime = false, want true")
	}
	if stats.Application.UIAssets != 1 || stats.Application.PackageAssets != 0 {
		t.Fatalf("manifest asset stats = %#v", stats.Application)
	}
}

func TestCollectCodingApplicationStatsRejectsInvalidManifest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "application.yaml")
	if err := os.WriteFile(path, []byte("roots: ["), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := collectCodingApplicationStats(path); err == nil {
		t.Fatal("collectCodingApplicationStats returned nil error for invalid YAML")
	}
}
