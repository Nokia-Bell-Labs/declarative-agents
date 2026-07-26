// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBootSmokeProfilesPassesWhenEveryProfilePreflights asserts the smoke calls
// the runtime preflight once per profile with the flags that reproduce a real
// startup, and reports success when each exits zero.
func TestBootSmokeProfilesPassesWhenEveryProfilePreflights(t *testing.T) {
	var calls [][]string
	run := func(binary string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{binary}, args...))
		return []byte("config valid"), nil
	}

	profiles := []string{"agents/monitor/profile.yaml", "agents/jurist/profile.yaml"}
	if err := bootSmokeProfiles(run, "/tmp/agent", "/core", profiles); err != nil {
		t.Fatalf("boot smoke should pass, got %v", err)
	}
	if len(calls) != len(profiles) {
		t.Fatalf("expected %d preflights, got %d", len(profiles), len(calls))
	}
	want := []string{"/tmp/agent", "--validate-config", "--profile", "agents/monitor/profile.yaml", "--core-root", "/core"}
	if strings.Join(calls[0], " ") != strings.Join(want, " ") {
		t.Errorf("preflight args = %v, want %v", calls[0], want)
	}
}

// TestBootSmokeProfilesReportsEveryFailure asserts a broken profile fails the
// audit, that the reported error carries the profile path and the runtime's own
// message, and that one failure does not stop the remaining profiles.
func TestBootSmokeProfilesReportsEveryFailure(t *testing.T) {
	run := func(_ string, args ...string) ([]byte, error) {
		profile := args[2]
		if strings.Contains(profile, "corpus-ingest") {
			return []byte(`Error: receipt-contract validation failed: tool "chroma_add" is declared reversible`), fmt.Errorf("exit status 1")
		}
		if strings.Contains(profile, "rest") {
			return []byte("Error: load REST definitions: unknown field \"descrption\""), fmt.Errorf("exit status 1")
		}
		return []byte("config valid"), nil
	}

	profiles := []string{
		"agents/knowledge-manager/corpus-ingest/profile.yaml",
		"agents/monitor/profile.yaml",
		"testdata/conformance/rest/profile.yaml",
	}
	err := bootSmokeProfiles(run, "/tmp/agent", "/core", profiles)
	if err == nil {
		t.Fatal("boot smoke should fail when a profile cannot preflight")
	}
	msg := err.Error()
	for _, want := range []string{
		"2 of 3 profile(s)",
		"corpus-ingest/profile.yaml",
		"receipt-contract validation failed",
		"testdata/conformance/rest/profile.yaml",
		"unknown field",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q missing %q", msg, want)
		}
	}
	// The healthy profile between the two failures must not appear as a failure.
	if strings.Contains(msg, "agents/monitor/profile.yaml") {
		t.Errorf("healthy profile reported as failing: %q", msg)
	}
}

// TestBootSmokeProfilesFallsBackToExitError asserts a preflight that fails with
// no output still reports the underlying error rather than an empty detail.
func TestBootSmokeProfilesFallsBackToExitError(t *testing.T) {
	run := func(_ string, _ ...string) ([]byte, error) {
		return nil, fmt.Errorf("fork/exec: permission denied")
	}
	err := bootSmokeProfiles(run, "/tmp/agent", "/core", []string{"agents/monitor/profile.yaml"})
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("expected the exec error to surface, got %v", err)
	}
}

// TestDiscoverAuditProfilesCoversAgentsAndConformanceFixtures asserts the smoke and the
// static validation govern the same set: shipped agents plus the optional
// conformance fixtures.
func TestDiscoverAuditProfilesCoversAgentsAndConformanceFixtures(t *testing.T) {
	root := t.TempDir()
	mkProfile(t, filepath.Join(root, "agents", "monitor", "profile.yaml"))
	mkProfile(t, filepath.Join(root, "testdata", "conformance", "rest", "profile.yaml"))

	profiles, err := discoverAuditProfiles(root)
	if err != nil {
		t.Fatalf("auditProfiles: %v", err)
	}
	if len(profiles) != 2 {
		t.Fatalf("expected 2 profiles, got %d: %v", len(profiles), profiles)
	}
}

// TestDiscoverAuditProfilesRequiresAgents asserts a root with no shipped agent profile
// is an error rather than a silently empty smoke.
func TestDiscoverAuditProfilesRequiresAgents(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := discoverAuditProfiles(root); err == nil {
		t.Fatal("expected an error when no profile-shaped file exists under agents")
	}
}

func mkProfile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("name: test\nmachine: machine.yaml\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}
