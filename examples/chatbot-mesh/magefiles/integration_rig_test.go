// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCollectorIntakeFilterScenario(t *testing.T) {
	exampleRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	coreRoot := envOrDefault("AGENT_CORE_ROOT", siblingPath(exampleRoot, "agent-core"))
	if !agentCoreAvailable(coreRoot) {
		t.Skipf("agent-core checkout not found at %s", coreRoot)
	}
	binary, err := buildAgent(coreRoot)
	if err != nil {
		t.Fatal(err)
	}
	workDir := t.TempDir()
	if err := runCollectorIntakeScenario(binary, coreRoot, exampleRoot, workDir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(workDir, "collector.ndjson")); err != nil {
		t.Fatalf("collector spool evidence: %v", err)
	}
}
