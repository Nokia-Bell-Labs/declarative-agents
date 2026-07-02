// Copyright (c) 2026 Nokia. All rights reserved.

package catalog

import (
	"path/filepath"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/pkg/spec"
)

func TestResolveProfilePathMapsInstalledCoreHome(t *testing.T) {
	coreHome := t.TempDir()
	spec.SetAgentCoreInstallRoot(coreHome)
	t.Cleanup(func() { spec.SetAgentCoreInstallRoot("") })

	got := resolveProfilePath("/profiles/agents/generator", "/opt/agent-core/tools/builtin/llm")
	want := filepath.Join(coreHome, "tools", "builtin", "llm")
	if got != want {
		t.Fatalf("resolveProfilePath = %q, want %q", got, want)
	}
}

func TestResolveProfilePathLeavesInstalledCorePathWithoutOverride(t *testing.T) {
	spec.SetAgentCoreInstallRoot("")
	t.Cleanup(func() { spec.SetAgentCoreInstallRoot("") })

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
