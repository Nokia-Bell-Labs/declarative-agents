// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// The charter is what decides which checks gate, so a change to the gate is a
// change to a checked-in document rather than to Go. This pins the
// traceability checks plus group-level-citation, which the corpus passed at
// its first load (GH-2352); removing any of them is a deliberate edit here.
func TestCodingAgentCharterGatesTheTraceabilityChecks(t *testing.T) {
	var charter struct {
		ID     string `yaml:"id"`
		Checks []struct {
			Kind   string   `yaml:"kind"`
			Checks []string `yaml:"checks"`
		} `yaml:"checks"`
	}
	path := filepath.Join("..", "docs", "corpus-charter.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if err := yaml.Unmarshal(data, &charter); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	if charter.ID != "coding-agent-corpus-charter" {
		t.Errorf("charter id = %q; charter ids must be unique across modules", charter.ID)
	}

	selected := map[string]bool{}
	for _, check := range charter.Checks {
		if check.Kind != "spec_corpus" {
			continue
		}
		for _, id := range check.Checks {
			selected[id] = true
		}
	}
	for _, want := range []string{"broken-touchpoint", "broken-citation", "group-level-citation"} {
		if !selected[want] {
			t.Errorf("charter %s does not gate %s", path, want)
		}
	}
}

// Audit must run the corpus audit; a build that drops the call would pass
// every gate while the specification graph rots.
func TestAuditRunsTheCorpusAudit(t *testing.T) {
	names := callsWithin(t, "magefile.go", "Audit")
	if !names["runCorpusAudit"] {
		t.Fatal("Audit does not call runCorpusAudit")
	}
}
