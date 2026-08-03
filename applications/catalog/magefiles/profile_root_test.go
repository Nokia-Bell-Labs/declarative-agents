// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveAgentCoreRootFromCatalog(t *testing.T) {
	repository := t.TempDir()
	catalogRoot := filepath.Join(repository, "applications", "catalog")
	coreRoot := filepath.Join(repository, "agent-core")

	got, err := resolveAgentCoreRoot(catalogRoot)
	if err != nil {
		t.Fatalf("resolveAgentCoreRoot returned error: %v", err)
	}
	if got != coreRoot {
		t.Fatalf("resolveAgentCoreRoot = %q, want %q", got, coreRoot)
	}
}

func TestResolveAgentCoreRootHonorsAbsoluteAndRelativeDemoConfig(t *testing.T) {
	catalogRoot := t.TempDir()
	absoluteRoot := filepath.Join(t.TempDir(), "agent-core")

	t.Run("absolute", func(t *testing.T) {
		writeCatalogDemoConfig(t, catalogRoot, "core_root: "+absoluteRoot+"\n")
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
		writeCatalogDemoConfig(t, catalogRoot, "core_root: test-core\n")
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
	writeCatalogDemoConfig(t, catalogRoot, "core_root: "+missingRoot+"\n")

	got, err := resolveAgentCoreRoot(catalogRoot)
	if err != nil {
		t.Fatalf("resolveAgentCoreRoot returned error: %v", err)
	}
	if got != missingRoot {
		t.Fatalf("resolveAgentCoreRoot = %q, want %q", got, missingRoot)
	}
}

func TestResolveAgentCoreImageFromDemoConfig(t *testing.T) {
	catalogRoot := t.TempDir()

	image, err := resolveAgentCoreImage(catalogRoot)
	if err != nil {
		t.Fatalf("resolveAgentCoreImage returned error: %v", err)
	}
	if image != defaultAgentCoreImage {
		t.Fatalf("resolveAgentCoreImage = %q, want %q", image, defaultAgentCoreImage)
	}

	writeCatalogDemoConfig(t, catalogRoot, "core_image: registry.example/agent-core:test\n")
	image, err = resolveAgentCoreImage(catalogRoot)
	if err != nil {
		t.Fatalf("resolveAgentCoreImage returned error: %v", err)
	}
	if image != "registry.example/agent-core:test" {
		t.Fatalf("resolveAgentCoreImage = %q, want configured image", image)
	}
}

func TestResolveAgentCoreRootRejectsMalformedDemoConfig(t *testing.T) {
	catalogRoot := t.TempDir()
	writeCatalogDemoConfig(t, catalogRoot, "core_root: [\n")

	if _, err := resolveAgentCoreRoot(catalogRoot); err == nil {
		t.Fatal("resolveAgentCoreRoot returned nil error for malformed demo.yaml")
	}
}

func writeCatalogDemoConfig(t *testing.T, root, contents string) {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, catalogDemoConfigFile), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
