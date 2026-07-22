// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBootSmokeProfilesPreflightsEveryMeshProfile asserts the smoke invokes the
// runtime preflight once per profile with the flags that reproduce a real start.
func TestBootSmokeProfilesPreflightsEveryMeshProfile(t *testing.T) {
	var calls [][]string
	run := func(binary string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{binary}, args...))
		return []byte("config valid"), nil
	}

	profiles := []string{"agents/chatbot/profile.yaml", "agents/rag-server/profile.yaml"}
	if err := bootSmokeProfiles(run, "/tmp/agent", "/core", profiles); err != nil {
		t.Fatalf("boot smoke should pass, got %v", err)
	}
	if len(calls) != len(profiles) {
		t.Fatalf("expected %d preflights, got %d", len(profiles), len(calls))
	}
	want := []string{"/tmp/agent", "--validate-config", "--profile", "agents/chatbot/profile.yaml", "--core-root", "/core"}
	if strings.Join(calls[0], " ") != strings.Join(want, " ") {
		t.Errorf("preflight args = %v, want %v", calls[0], want)
	}
}

// TestBootSmokeProfilesFailsAuditOnUnbootableProfile asserts a profile the
// runtime rejects fails the audit and that the runtime's message is reported.
func TestBootSmokeProfilesFailsAuditOnUnbootableProfile(t *testing.T) {
	run := func(_ string, args ...string) ([]byte, error) {
		if strings.Contains(args[2], "chatbot") {
			return []byte(`Error: receipt-contract validation failed: tool "await_control" is declared reversible`), fmt.Errorf("exit status 1")
		}
		return []byte("config valid"), nil
	}

	profiles := []string{"agents/chatbot/profile.yaml", "agents/creator/profile.yaml"}
	err := bootSmokeProfiles(run, "/tmp/agent", "/core", profiles)
	if err == nil {
		t.Fatal("boot smoke should fail when a mesh profile cannot preflight")
	}
	for _, want := range []string{"1 of 2 profile(s)", "agents/chatbot/profile.yaml", "receipt-contract validation failed"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err.Error(), want)
		}
	}
	if strings.Contains(err.Error(), "agents/creator/profile.yaml") {
		t.Errorf("healthy profile reported as failing: %q", err.Error())
	}
}

// TestMeshProfilesFindsShippedAgents asserts discovery collects the agent
// profiles and fails loudly on a root with none, so the smoke cannot pass by
// silently checking nothing.
func TestMeshProfilesFindsShippedAgents(t *testing.T) {
	root := t.TempDir()
	for _, agent := range []string{"chatbot", "rag-server"} {
		dir := filepath.Join(root, "agents", agent)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "profile.yaml"), []byte("name: x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	profiles, err := meshProfiles(root)
	if err != nil {
		t.Fatalf("meshProfiles: %v", err)
	}
	if len(profiles) != 2 {
		t.Fatalf("expected 2 profiles, got %d: %v", len(profiles), profiles)
	}

	empty := t.TempDir()
	if err := os.MkdirAll(filepath.Join(empty, "agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := meshProfiles(empty); err == nil {
		t.Fatal("expected an error when no agent profile exists")
	}
}
