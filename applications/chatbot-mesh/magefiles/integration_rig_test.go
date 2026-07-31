// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCollectorIntakeFilterScenario(t *testing.T) {
	applicationRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	catalogRoot, err := resolveCatalogRoot("collector intake test", applicationRoot)
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
	if err := runCollectorIntakeScenario(binary, coreRoot, catalogRoot, workDir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(workDir, "collector.ndjson")); err != nil {
		t.Fatalf("collector spool evidence: %v", err)
	}
}

func TestCollectorLifecycleRebindAndTerminalState(t *testing.T) {
	applicationRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	catalogRoot, err := resolveCatalogRoot("collector lifecycle test", applicationRoot)
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
	result, err := runCollectorLifecycleScenario(binary, coreRoot, catalogRoot, workDir)
	if err != nil {
		t.Fatal(err)
	}
	if !result.MonitorReachable {
		t.Error("monitor was not reachable while collector was running")
	}
	if result.TerminalState != "succeeded" {
		t.Errorf("terminal state = %q, want %q", result.TerminalState, "succeeded")
	}
	if !result.AllAddrsRebind {
		t.Error("not all listener addresses could rebind after exit")
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

func TestMeshScenarioCriticIdentities(t *testing.T) {
	tests := []struct {
		scenario, identity string
	}{
		{"agents/chatbot/tests/single-turn", "chatbot-turn-critic"},
		{"agents/chatbot/tests/degraded-rag", "chatbot-turn-critic"},
		{"agents/rag-server/tests/query", "rag-query-critic"},
	}
	for _, test := range tests {
		t.Run(test.scenario, func(t *testing.T) {
			for _, file := range []string{"profile.yaml", "machine.yaml"} {
				data, err := os.ReadFile(filepath.Join("..", filepath.FromSlash(test.scenario), file))
				if err != nil {
					t.Fatal(err)
				}
				if !strings.HasPrefix(string(data), "name: "+test.identity+"\n") {
					t.Fatalf("%s/%s does not declare %q:\n%s", test.scenario, file, test.identity, data)
				}
			}
		})
	}
}
