// Copyright (c) 2026 Nokia. All rights reserved.

package profiles

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// profileCheckout builds a directory that looks like an agent-profiles
// checkout: an agents/ directory holding more than one shipped profile.
func profileCheckout(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"executor", "critic"} {
		agent := filepath.Join(dir, "agents", name)
		require.NoError(t, os.MkdirAll(agent, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(agent, "profile.yaml"), []byte("machine: machine.yaml\n"), 0o644))
	}
	return dir
}

// partialCheckout holds a single profile. One profile is not a checkout: a
// stray directory with the right name should not be mistaken for one.
func partialCheckout(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	agent := filepath.Join(dir, "agents", "executor")
	require.NoError(t, os.MkdirAll(agent, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(agent, "profile.yaml"), []byte("machine: machine.yaml\n"), 0o644))
	return dir
}

func TestResolvePolicy(t *testing.T) {
	valid := profileCheckout(t)
	discovered := profileCheckout(t)
	empty := t.TempDir()

	tests := []struct {
		name        string
		configured  string
		candidates  []string
		wantOutcome Outcome
		wantPath    string
		wantSource  string
	}{
		{
			name:        "configured root wins over a discovered checkout",
			configured:  valid,
			candidates:  []string{discovered},
			wantOutcome: Found,
			wantPath:    valid,
			wantSource:  RootEnv,
		},
		{
			name:        "discovery runs when nothing is configured",
			candidates:  []string{empty, discovered},
			wantOutcome: Found,
			wantPath:    discovered,
			wantSource:  "discovered checkout",
		},
		{
			name:        "blank configuration is treated as unset",
			configured:  "   ",
			candidates:  []string{discovered},
			wantOutcome: Found,
			wantPath:    discovered,
			wantSource:  "discovered checkout",
		},
		{
			name:        "a configured root holding no profiles is an error",
			configured:  empty,
			candidates:  []string{discovered},
			wantOutcome: Invalid,
			wantPath:    empty,
			wantSource:  RootEnv,
		},
		{
			name:        "nothing configured and nothing found is absent",
			candidates:  []string{empty},
			wantOutcome: Absent,
		},
		{
			name:        "no candidates at all is absent",
			wantOutcome: Absent,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := Resolve(tc.configured, tc.candidates...)
			require.Equal(t, tc.wantOutcome, res.Outcome)
			require.Equal(t, tc.wantPath, res.Path)
			require.Equal(t, tc.wantSource, res.Source)
		})
	}
}

// TestResolveAcceptsAgentsDirectory covers the caller that points at the
// agents/ directory rather than the repository root. Both name the same
// checkout, so both resolve to the repository root.
func TestResolveAcceptsAgentsDirectory(t *testing.T) {
	root := profileCheckout(t)
	res := Resolve(filepath.Join(root, "agents"))
	require.Equal(t, Found, res.Outcome)
	require.Equal(t, root, res.Path)
}

// TestResolveRejectsPartialCheckout keeps a directory holding one profile from
// reading as a checkout, so a half-populated path fails loudly at resolution
// rather than at the first missing profile.
func TestResolveRejectsPartialCheckout(t *testing.T) {
	res := Resolve(partialCheckout(t))
	require.Equal(t, Invalid, res.Outcome)
}

// TestReasonNamesTheRemedy is what a skipping caller prints. An absent
// prerequisite has to say how to supply it (srd034 R3.4).
func TestReasonNamesTheRemedy(t *testing.T) {
	require.Contains(t, Resolve("").Reason(), RootEnv)
	require.Contains(t, Resolve(t.TempDir()).Reason(), RootEnv)
	require.Empty(t, Resolve(profileCheckout(t)).Reason())
}

// TestResolveFromFindsSiblingCheckout proves the standard candidate order finds
// a checkout beside the module, which is the documented layout.
func TestResolveFromFindsSiblingCheckout(t *testing.T) {
	t.Setenv(RootEnv, "")
	parent := t.TempDir()
	moduleRoot := filepath.Join(parent, "agent-core")
	require.NoError(t, os.MkdirAll(moduleRoot, 0o755))
	for _, name := range []string{"executor", "critic"} {
		agent := filepath.Join(parent, "agent-profiles", "agents", name)
		require.NoError(t, os.MkdirAll(agent, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(agent, "profile.yaml"), []byte("machine: machine.yaml\n"), 0o644))
	}

	res := ResolveFrom(moduleRoot)
	require.Equal(t, Found, res.Outcome)
	require.Equal(t, filepath.Join(parent, "agent-profiles"), res.Path)
	require.Equal(t, filepath.Join(res.Path, "agents"), res.AgentsRoot())
	require.Equal(t, filepath.Join(res.Path, "testdata", "conformance"), res.ConformanceRoot())
}
