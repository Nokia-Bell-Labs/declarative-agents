// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCollectorIntakeFilterScenario(t *testing.T) {
	applicationRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	coreRoot := envOrDefault("AGENT_CORE_ROOT", siblingPath(applicationRoot, "agent-core"))
	if !agentCoreAvailable(coreRoot) {
		t.Skipf("agent-core checkout not found at %s", coreRoot)
	}
	binary, err := buildAgent(coreRoot)
	if err != nil {
		t.Fatal(err)
	}
	workDir := t.TempDir()
	if err := runCollectorIntakeScenario(binary, coreRoot, applicationRoot, workDir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(workDir, "collector.ndjson")); err != nil {
		t.Fatalf("collector spool evidence: %v", err)
	}
}

func TestStageRigRuntimeUsesCatalogScenarioCritic(t *testing.T) {
	applicationRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	catalogRoot, err := filepath.Abs(filepath.Join("..", "..", "catalog"))
	if err != nil {
		t.Fatal(err)
	}
	stage, cleanup, err := stageRigRuntime(applicationRoot, catalogRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	for _, path := range []string{
		"agents/scenario-critic/machine.yaml",
		"agents/scenario-critic/tools.yaml",
		"testdata/rig/declarations.yaml",
	} {
		if _, err := os.Stat(filepath.Join(stage, filepath.FromSlash(path))); err != nil {
			t.Errorf("staged rig missing %s: %v", path, err)
		}
	}
	profile, err := os.ReadFile(filepath.Join(stage, filepath.FromSlash(rigProfile)))
	if err != nil {
		t.Fatal(err)
	}
	if string(profile) != "name: scenario-critic\nmachine: ../../agents/scenario-critic/machine.yaml\ntools:\n  - ../../agents/scenario-critic/tools.yaml\ntool_declarations:\n  - declarations.yaml\nrest_definitions:\n  - rest.yaml\n" {
		t.Fatalf("staged rig profile does not select catalog scenario critic:\n%s", profile)
	}
}
