// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"testing"
)

const testAgentCoreModule = "github.com/Nokia-Bell-Labs/declarative-agents/agent-core"

func TestResolveAgentCoreRootFromCatalog(t *testing.T) {
	repository := t.TempDir()
	catalogRoot := filepath.Join(repository, "applications", "catalog")
	coreRoot := filepath.Join(repository, "agent-core")
	writeModule(t, coreRoot, testAgentCoreModule)
	t.Setenv(agentCoreRootEnv, "")

	got, err := resolveAgentCoreRoot(catalogRoot)
	if err != nil {
		t.Fatalf("resolveAgentCoreRoot returned error: %v", err)
	}
	if got != coreRoot {
		t.Fatalf("resolveAgentCoreRoot = %q, want %q", got, coreRoot)
	}
}

func TestResolveAgentCoreRootHonorsAbsoluteAndRelativeEnvironment(t *testing.T) {
	catalogRoot := t.TempDir()
	absoluteRoot := filepath.Join(t.TempDir(), "agent-core")
	writeModule(t, absoluteRoot, testAgentCoreModule)

	t.Run("absolute", func(t *testing.T) {
		t.Setenv(agentCoreRootEnv, absoluteRoot)
		got, err := resolveAgentCoreRoot(catalogRoot)
		if err != nil {
			t.Fatalf("resolveAgentCoreRoot returned error: %v", err)
		}
		if got != absoluteRoot {
			t.Fatalf("resolveAgentCoreRoot = %q, want %q", got, absoluteRoot)
		}
	})

	t.Run("relative to owner root", func(t *testing.T) {
		relativeRoot := filepath.Join(catalogRoot, "test-core")
		writeModule(t, relativeRoot, testAgentCoreModule)
		t.Setenv(agentCoreRootEnv, "test-core")
		got, err := resolveAgentCoreRoot(catalogRoot)
		if err != nil {
			t.Fatalf("resolveAgentCoreRoot returned error: %v", err)
		}
		if got != relativeRoot {
			t.Fatalf("resolveAgentCoreRoot = %q, want %q", got, relativeRoot)
		}
	})
}

func TestResolveAgentCoreRootDoesNotRequireCheckoutForSkipSemantics(t *testing.T) {
	catalogRoot := t.TempDir()
	missingRoot := filepath.Join(t.TempDir(), "missing-core")
	t.Setenv(agentCoreRootEnv, missingRoot)

	got, err := resolveAgentCoreRoot(catalogRoot)
	if err != nil {
		t.Fatalf("resolveAgentCoreRoot returned error: %v", err)
	}
	if got != missingRoot {
		t.Fatalf("resolveAgentCoreRoot = %q, want %q", got, missingRoot)
	}
}

func writeModule(t *testing.T, root, module string) {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module "+module+"\n\ngo 1.26.3\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
}
