// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"reflect"
	"testing"
)

// TestExampleModulesParticipateInAudit proves the example modules are dispatched
// by the root audit gate alongside the platform sub-modules, so a standalone
// example cannot silently drop out of mage audit.
func TestExampleModulesParticipateInAudit(t *testing.T) {
	participants := auditParticipants()
	for _, mod := range exampleModules {
		if !contains(participants, mod) {
			t.Fatalf("auditParticipants() = %#v, missing example module %q", participants, mod)
		}
	}
	for _, mod := range subModules {
		if !contains(participants, mod) {
			t.Fatalf("auditParticipants() = %#v, missing sub-module %q", participants, mod)
		}
	}
}

// TestChatbotMeshIsAnExampleModule pins the mesh module into the example gate so
// the #476 regression (root gates omitting examples/chatbot-mesh) stays fixed.
func TestChatbotMeshIsAnExampleModule(t *testing.T) {
	if !contains(exampleModules, "examples/chatbot-mesh") {
		t.Fatalf("exampleModules = %#v, want it to include examples/chatbot-mesh", exampleModules)
	}
}

// TestExampleModulesExcludedFromSubModules proves example modules do not enter
// the Build, Stats, and All gates, which iterate subModules and would fail on a
// module that defines no build/stats/default target.
func TestExampleModulesExcludedFromSubModules(t *testing.T) {
	for _, mod := range exampleModules {
		if contains(subModules, mod) {
			t.Fatalf("subModules must not contain example module %q (it has no build/stats target)", mod)
		}
	}
}

// TestTestSubModulesDispatchesExampleModules proves the go-test dispatch path
// visits every example module that owns Go tests.
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
		t.Fatalf("tested example modules = %#v, want %#v", got, exampleModules)
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
