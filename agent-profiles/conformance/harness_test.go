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

// TestResolveCoreRootPolicy covers the documented prerequisite: AGENT_CORE_ROOT
// or a sibling checkout. An explicitly configured root wins and must be usable;
// an absent prerequisite skips rather than fails, so a docs-only checkout keeps
// `go test ./...` hermetic (GH-584).
func TestResolveCoreRootPolicy(t *testing.T) {
	valid := coreCheckout(t)
	sibling := coreCheckout(t)
	empty := t.TempDir() // exists but holds no go.mod

	tests := []struct {
		name        string
		env         string
		sibling     string
		wantOutcome coreRootOutcome
		wantPath    string
		wantSource  string
	}{
		{
			name:        "environment path is honored over the sibling",
			env:         valid,
			sibling:     sibling,
			wantOutcome: coreRootFound,
			wantPath:    valid,
			wantSource:  AgentCoreRootEnv,
		},
		{
			name:        "sibling fallback when the environment is unset",
			env:         "",
			sibling:     sibling,
			wantOutcome: coreRootFound,
			wantPath:    sibling,
			wantSource:  "sibling checkout",
		},
		{
			name:        "blank environment is treated as unset",
			env:         "   ",
			sibling:     sibling,
			wantOutcome: coreRootFound,
			wantPath:    sibling,
			wantSource:  "sibling checkout",
		},
		{
			name:        "invalid environment path fails rather than silently skipping",
			env:         empty,
			sibling:     sibling,
			wantOutcome: coreRootInvalid,
			wantPath:    empty,
			wantSource:  AgentCoreRootEnv,
		},
		{
			name:        "nonexistent environment path fails even when a sibling exists",
			env:         filepath.Join(empty, "does-not-exist"),
			sibling:     sibling,
			wantOutcome: coreRootInvalid,
			wantSource:  AgentCoreRootEnv,
		},
		{
			name:        "absent prerequisite skips",
			env:         "",
			sibling:     filepath.Join(empty, "agent-core"),
			wantOutcome: coreRootAbsent,
			wantSource:  "sibling checkout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveCoreRoot(tt.env, tt.sibling)
			if got.Outcome != tt.wantOutcome {
				t.Fatalf("outcome = %v, want %v", got.Outcome, tt.wantOutcome)
			}
			if tt.wantPath != "" && got.Path != tt.wantPath {
				t.Errorf("path = %q, want %q", got.Path, tt.wantPath)
			}
			if got.Source != tt.wantSource {
				t.Errorf("source = %q, want %q", got.Source, tt.wantSource)
			}
		})
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
	if got := resolveCoreRoot(dir, ""); got.Outcome != coreRootInvalid {
		t.Errorf("outcome = %v, want coreRootInvalid", got.Outcome)
	}
	if isCoreCheckout(dir) {
		t.Error("a directory named go.mod must not count as a checkout")
	}
}

// TestRequireCoreRootHonorsEnvironment asserts the exported helper resolves
// through the environment, which is the behavior the formal suites document and
// the defect this issue reports.
func TestRequireCoreRootHonorsEnvironment(t *testing.T) {
	want := coreCheckout(t)
	t.Setenv(AgentCoreRootEnv, want)

	got := RequireCoreRoot(t)
	abs, err := filepath.Abs(want)
	if err != nil {
		t.Fatal(err)
	}
	if got != abs {
		t.Errorf("RequireCoreRoot() = %q, want %q", got, abs)
	}
}
