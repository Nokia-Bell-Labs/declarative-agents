// Copyright (c) 2026 Nokia. All rights reserved.

package catalog

import (
	"path/filepath"
	"testing"
)

func TestResolveProfilePathMapsInstalledCoreHome(t *testing.T) {
	coreHome := t.TempDir()
	t.Setenv(agentCoreHomeEnv, coreHome)

	got := resolveProfilePath("/profiles/agents/generator", "/opt/agent-core/tools/builtin/llm")
	want := filepath.Join(coreHome, "tools", "builtin", "llm")
	if got != want {
		t.Fatalf("resolveProfilePath = %q, want %q", got, want)
	}
}

func TestResolveProfilePathLeavesInstalledCorePathWithoutOverride(t *testing.T) {
	t.Setenv(agentCoreHomeEnv, "")

	got := resolveProfilePath("/profiles/agents/generator", "/opt/agent-core/tools/builtin/llm")
	want := "/opt/agent-core/tools/builtin/llm"
	if got != want {
		t.Fatalf("resolveProfilePath = %q, want %q", got, want)
	}
}

func TestResolveProfilePathKeepsRelativeProfilePaths(t *testing.T) {
	t.Parallel()

	got := resolveProfilePath("/profiles/agents/generator", "machine.yaml")
	want := filepath.Join("/profiles/agents/generator", "machine.yaml")
	if got != want {
		t.Fatalf("resolveProfilePath = %q, want %q", got, want)
	}
}
