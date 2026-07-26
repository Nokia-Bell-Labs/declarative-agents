// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCollectCodingApplicationStatsReportsCompositionWithoutAgents(t *testing.T) {
	manifest := `agent_profiles:
  references:
    - role: planner
    - role: executor
    - role: critic
    - role: critic-workspace
runtime:
  image_contains_profiles: false
deployment:
  serving_profiles:
    - role: planner
    - role: executor
    - role: critic
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
	if stats.Application.CanonicalReferences != 4 ||
		!reflect.DeepEqual(stats.Application.CanonicalRoles,
			[]string{"planner", "executor", "critic", "critic-workspace"}) {
		t.Errorf("canonical references = %d %#v",
			stats.Application.CanonicalReferences, stats.Application.CanonicalRoles)
	}
	if stats.Application.ServingProfiles != 3 ||
		!reflect.DeepEqual(stats.Application.ServingRoles,
			[]string{"planner", "executor", "critic"}) {
		t.Errorf("serving profiles = %d %#v",
			stats.Application.ServingProfiles, stats.Application.ServingRoles)
	}
	if !stats.Application.ProfileFreeRuntime {
		t.Error("profile_free_runtime = false, want true")
	}
}

func TestCollectCodingApplicationStatsRejectsInvalidManifest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "application.yaml")
	if err := os.WriteFile(path, []byte("agent_profiles: ["), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := collectCodingApplicationStats(path); err == nil {
		t.Fatal("collectCodingApplicationStats returned nil error for invalid YAML")
	}
}
