// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"os"
	"path/filepath"
	"testing"
)

// coreCheckout creates a directory that looks like an agent-core module.
func coreCheckout(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/agent-core\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestResolveCoreRootPolicy covers the documented monorepo prerequisite. An
// absent checkout skips rather than fails, so a docs-only checkout keeps `go
// test ./...` hermetic (GH-584).
func TestResolveCoreRootPolicy(t *testing.T) {
	repositoryCheckout := coreCheckout(t)
	empty := t.TempDir() // exists but holds no go.mod

	tests := []struct {
		name        string
		repository  string
		wantOutcome coreRootOutcome
		wantPath    string
	}{
		{
			name:        "repository checkout is discovered",
			repository:  repositoryCheckout,
			wantOutcome: coreRootFound,
			wantPath:    repositoryCheckout,
		},
		{
			name:        "absent prerequisite skips",
			repository:  filepath.Join(empty, "agent-core"),
			wantOutcome: coreRootAbsent,
			wantPath:    filepath.Join(empty, "agent-core"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveCoreRoot(tt.repository)
			if got.Outcome != tt.wantOutcome {
				t.Fatalf("outcome = %v, want %v", got.Outcome, tt.wantOutcome)
			}
			if tt.wantPath != "" && got.Path != tt.wantPath {
				t.Errorf("path = %q, want %q", got.Path, tt.wantPath)
			}
		})
	}
}

func TestAgentCoreRepositoryPathFromApplicationsCatalog(t *testing.T) {
	repository := t.TempDir()
	catalogRoot := filepath.Join(repository, "applications", "catalog")
	if got := agentCoreRepositoryPath(catalogRoot); got != filepath.Join(repository, "agent-core") {
		t.Fatalf("agentCoreRepositoryPath = %q, want %q", got, filepath.Join(repository, "agent-core"))
	}
}

// TestResolveCoreRootRejectsDirectoryNamedGoMod guards the checkout probe: a
// directory called go.mod is not a module, so it must not satisfy the
// prerequisite.
func TestResolveCoreRootRejectsDirectoryNamedGoMod(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "go.mod"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := resolveCoreRoot(dir); got.Outcome != coreRootAbsent {
		t.Errorf("outcome = %v, want coreRootAbsent", got.Outcome)
	}
	if isCoreCheckout(dir) {
		t.Error("a directory named go.mod must not count as a checkout")
	}
}
