// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestExampleModulesParticipateInAudit proves the application modules are dispatched
// by the root audit gate alongside the platform sub-modules, so a standalone
// application cannot silently drop out of mage audit.
func TestExampleModulesParticipateInAudit(t *testing.T) {
	participants := auditParticipants()
	for _, mod := range exampleModules {
		if !contains(participants, mod) {
			t.Fatalf("auditParticipants() = %#v, missing application module %q", participants, mod)
		}
	}
	for _, mod := range subModules {
		if !contains(participants, mod) {
			t.Fatalf("auditParticipants() = %#v, missing sub-module %q", participants, mod)
		}
	}
}

// TestChatbotMeshIsAnExampleModule pins the mesh module into the application gate
// so the #476 regression (root gates omitting applications/chatbot-mesh) stays fixed.
func TestChatbotMeshIsAnExampleModule(t *testing.T) {
	if !contains(exampleModules, "applications/chatbot-mesh") {
		t.Fatalf("exampleModules = %#v, want it to include applications/chatbot-mesh", exampleModules)
	}
}

func TestCodingAgentParticipatesInAudit(t *testing.T) {
	if !contains(auditParticipants(), "applications/coding-agent") {
		t.Fatalf("auditParticipants() = %#v, want coding-agent", auditParticipants())
	}
	if !contains(exampleModules, "applications/coding-agent") {
		t.Fatal("coding-agent owns tests and stats and must participate as an application module")
	}
}

func TestEveryOrchestratedModuleDirectoryExists(t *testing.T) {
	modules := append(append([]string{}, subModules...), exampleModules...)
	for _, module := range modules {
		info, err := os.Stat(filepath.Join("..", filepath.FromSlash(module)))
		if err != nil {
			t.Errorf("orchestrated module %s does not resolve to a directory: %v", module, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("indexed module %s is not a directory", module)
		}
	}
}

// TestExampleModulesExcludedFromSubModules proves application modules do not enter
// the Build and All gates, which iterate subModules and would fail on a module
// that defines no build/default target.
func TestExampleModulesExcludedFromSubModules(t *testing.T) {
	for _, mod := range exampleModules {
		if contains(subModules, mod) {
			t.Fatalf("subModules must not contain application module %q (it has no build target)", mod)
		}
	}
}

// TestStatsParticipantsIncludeExampleModules proves the root stats gate dispatches
// to the application modules, so the repo-wide agents total cannot silently drop
// application agents (GH-754).
func TestStatsParticipantsIncludeExampleModules(t *testing.T) {
	participants := statsParticipants()
	for _, mod := range exampleModules {
		if !contains(participants, mod) {
			t.Fatalf("statsParticipants() = %#v, missing application module %q", participants, mod)
		}
	}
	for _, mod := range subModules {
		if !contains(participants, mod) {
			t.Fatalf("statsParticipants() = %#v, missing sub-module %q", participants, mod)
		}
	}
}

// TestTestSubModulesDispatchesExampleModules proves the go-test dispatch path
// visits every application module that owns Go tests.
func TestTestSubModulesDispatchesExampleModules(t *testing.T) {
	var got []string
	err := testSubModules(
		exampleModules,
		func(string) (bool, error) { return true, nil },
		func(dir string) error {
			got = append(got, dir)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("testSubModules returned error: %v", err)
	}
	if !reflect.DeepEqual(got, exampleModules) {
		t.Fatalf("tested application modules = %#v, want %#v", got, exampleModules)
	}
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
