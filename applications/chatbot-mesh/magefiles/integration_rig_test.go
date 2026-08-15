// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func yamlDocument(data []byte) string {
	return strings.TrimPrefix(string(data), "# Copyright (c) 2026 Nokia\n# SPDX-License-Identifier: BSD-3-Clause\n")
}

func TestCollectorIntakeFilterScenario(t *testing.T) {
	applicationRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	catalogRoot, err := resolveCatalogRoot("collector intake test", applicationRoot)
	if err != nil {
		t.Fatal(err)
	}
	coreRoot := demoCoreRoot(applicationRoot)
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
	coreRoot := demoCoreRoot(applicationRoot)
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
	stage, cleanup, err := stageRigRuntime(applicationRoot, catalogRoot, "127.0.0.1:4317")
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
	if yamlDocument(profile) != "name: scenario-critic\nmachine: ../../agents/scenario-critic/machine.yaml\ntools:\n  - ../../agents/scenario-critic/tools.yaml\ntool_declarations:\n  - declarations.yaml\nrest_definitions:\n  - rest.yaml\n" {
		t.Fatalf("staged rig profile does not select catalog scenario critic:\n%s", profile)
	}
	declarations, err := os.ReadFile(filepath.Join(stage, "testdata", "rig", "declarations.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(declarations), `otlp_endpoint: "127.0.0.1:4317"`) {
		t.Fatalf("staged rig declarations do not configure the validator OTLP endpoint:\n%s", declarations)
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
				if !strings.HasPrefix(yamlDocument(data), "name: "+test.identity+"\n") {
					t.Fatalf("%s/%s does not declare %q:\n%s", test.scenario, file, test.identity, data)
				}
			}
		})
	}
}

// TestRigMockPortsMatchScenarioFixtures guards the preflight against drift: the
// ports it refuses to run over must be exactly the ones the scenarios pin their
// mock fixtures to. Repin a fixture without updating the preflight and this
// fails, before a developer rediscovers GH-1229 the hard way.
func TestRigMockPortsMatchScenarioFixtures(t *testing.T) {
	scenarios := []string{
		"agents/chatbot/tests/single-turn/scenario.yaml",
		"agents/chatbot/tests/degraded-rag/scenario.yaml",
	}
	for _, mock := range rigMockFixturePorts {
		for _, scenario := range scenarios {
			data, err := os.ReadFile(filepath.Join("..", filepath.FromSlash(scenario)))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(data), "127.0.0.1:"+mock.port) {
				t.Errorf("%s does not pin a fixture to preflight port %s (%s)", scenario, mock.port, mock.name)
			}
		}
	}
}

// TestRigMockPortSkipReason confirms the preflight skips when a fixture port is
// held and clears once it is free — the non-blocking behaviour GH-1229 asked
// for. It injects a self-bound port so the check is hermetic and never depends
// on whether a real Ollama happens to be running on the host.
func TestRigMockPortSkipReason(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}

	saved := rigMockFixturePorts
	defer func() { rigMockFixturePorts = saved }()
	rigMockFixturePorts = []struct{ name, port string }{{"test mock fixture", port}}

	if reason := rigMockPortSkipReason(); reason == "" {
		t.Fatalf("expected a skip reason while port %s is held", port)
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	if reason := rigMockPortSkipReason(); reason != "" {
		t.Fatalf("expected no skip reason once port %s is free, got: %s", port, reason)
	}
}
