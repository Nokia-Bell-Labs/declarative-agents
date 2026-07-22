// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestJuristSucceeded pins the report classification to the jurist's observed
// output contract: a clean run ends "terminal state: succeeded"; a failing run
// ends "terminal state: failed" (with "status=failed" in the run-complete log);
// both exit zero, so the terminal state is the only signal. A report with neither
// marker is an indeterminate run and must be an error, not a silent pass.
func TestJuristSucceeded(t *testing.T) {
	cases := []struct {
		name    string
		report  string
		wantOK  bool
		wantErr bool
	}{
		{
			name:   "clean corpus",
			report: "validate: 3 SRDs ... — OK\nrun complete: status=succeeded\nterminal state: succeeded\n",
			wantOK: true,
		},
		{
			name:    "error finding fails",
			report:  "[error] builtin-spec-corpus/index-broken-path ...\nrun complete: status=failed\nterminal state: failed\n",
			wantOK:  false,
			wantErr: false,
		},
		{
			name:    "status=failed without terminal line still fails",
			report:  "run complete: status=failed iterations=3\n",
			wantOK:  false,
			wantErr: false,
		},
		{
			name:   "warnings only still succeed",
			report: "[warning] builtin-spec-corpus/orphaned-srd ...\nterminal state: succeeded\n",
			wantOK: true,
		},
		{
			name:    "indeterminate run is an error",
			report:  "building agent binary...\n",
			wantOK:  false,
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ok, err := juristSucceeded(tc.report)
			if ok != tc.wantOK {
				t.Errorf("ok = %v, want %v", ok, tc.wantOK)
			}
			if (err != nil) != tc.wantErr {
				t.Errorf("err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

// TestResolveAuditToolsRequiresRuntimeAndValidator pins the self-governance gate:
// a copied-out example that cannot reach the agent-core runtime or the jurist
// validator profile must fail, not skip to a false green. Only when both tools
// are present does resolution succeed.
func TestResolveAuditToolsRequiresRuntimeAndValidator(t *testing.T) {
	t.Run("missing agent-core runtime fails", func(t *testing.T) {
		t.Setenv(agentCoreRootEnv, filepath.Join(t.TempDir(), "absent-core"))
		t.Setenv(juristProfileEnv, writeFile(t, "profile.yaml", "name: fake-jurist\n"))
		if _, _, err := resolveAuditTools(t.TempDir()); err == nil {
			t.Fatal("expected an error when agent-core is absent, got nil")
		}
	})
	t.Run("missing jurist validator fails", func(t *testing.T) {
		t.Setenv(agentCoreRootEnv, fakeCore(t))
		t.Setenv(juristProfileEnv, filepath.Join(t.TempDir(), "absent-profile.yaml"))
		if _, _, err := resolveAuditTools(t.TempDir()); err == nil {
			t.Fatal("expected an error when the jurist validator is absent, got nil")
		}
	})
	t.Run("both present resolves", func(t *testing.T) {
		core := fakeCore(t)
		profile := writeFile(t, "profile.yaml", "name: fake-jurist\n")
		t.Setenv(agentCoreRootEnv, core)
		t.Setenv(juristProfileEnv, profile)
		coreRoot, juristProfile, err := resolveAuditTools(t.TempDir())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if coreRoot != core || juristProfile != profile {
			t.Fatalf("resolved (%s, %s), want (%s, %s)", coreRoot, juristProfile, core, profile)
		}
	})
}

// fakeCore returns a temp directory that agentCoreAvailable accepts as an
// agent-core module checkout (it carries a go.mod file).
func fakeCore(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module fake\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// writeFile writes content to a named file in a fresh temp directory and returns
// its path.
func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
