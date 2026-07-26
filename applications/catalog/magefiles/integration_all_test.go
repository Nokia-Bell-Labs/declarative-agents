// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"path/filepath"
	"testing"
)

func TestAgentCoreCheckoutAvailable(t *testing.T) {
	root := t.TempDir()
	if agentCoreCheckoutAvailable(root) {
		t.Fatalf("expected checkout unavailable before go.mod is written")
	}

	writeFile(t, filepath.Join(root, "go.mod"), "module example.com/agent-core\n")
	if !agentCoreCheckoutAvailable(root) {
		t.Fatalf("expected checkout available after go.mod is written")
	}
}

func TestAgentCoreCheckoutAvailableRejectsGoModDir(t *testing.T) {
	root := t.TempDir()
	mkdir(t, filepath.Join(root, "go.mod"))
	if agentCoreCheckoutAvailable(root) {
		t.Fatalf("expected a go.mod directory not to count as a checkout")
	}
}
