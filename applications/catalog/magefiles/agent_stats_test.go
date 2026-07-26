// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeAgentFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

const fixtureMachine = `name: alpha
states:
- name: Idle
- name: Working
- name: Done
transitions:
- state: Idle
  signal: Seed
  next: Working
- state: Working
  signal: ToolDone
  next: Done
`

const fixtureRequestMachine = `name: alpha-request
states:
- name: Waiting
transitions:
- state: Waiting
  signal: Seed
  next: Waiting
`

const fixtureTools = `tools:
  - load_corpus
  - format_report
`

const fixtureDeclarations = `tools:
  - name: load_corpus
    type: builtin
  - name: format_report
    type: builtin
  - name: extra_declared_only
    type: builtin
`

const fixtureProfile = `name: alpha
machine: machine.yaml
tools:
  - tools.yaml
`

// TestScanAgents proves the per-agent counts: states and transitions sum
// across every *machine.yaml file, tools count only from tools.yaml (not
// declarations.yaml or profile.yaml), and YAML lines cover every YAML file
// recursively. README-only agent directories are skipped.
func TestScanAgents(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	agentsDir := filepath.Join(root, "agents")

	writeAgentFixture(t, filepath.Join(agentsDir, "alpha", "machine.yaml"), fixtureMachine)
	writeAgentFixture(t, filepath.Join(agentsDir, "alpha", "request-machine.yaml"), fixtureRequestMachine)
	writeAgentFixture(t, filepath.Join(agentsDir, "alpha", "tools.yaml"), fixtureTools)
	writeAgentFixture(t, filepath.Join(agentsDir, "alpha", "declarations.yaml"), fixtureDeclarations)
	writeAgentFixture(t, filepath.Join(agentsDir, "alpha", "profile.yaml"), fixtureProfile)
	writeAgentFixture(t, filepath.Join(agentsDir, "alpha", "suites", "extra.yaml"), "suite: smoke\n")
	writeAgentFixture(t, filepath.Join(agentsDir, "readme-only", "README.md"), "# placeholder\n")

	section, err := scanAgents(agentsDir, profileCountLines)
	if err != nil {
		t.Fatalf("scanAgents returned error: %v", err)
	}

	if section.Total.Agents != 1 {
		t.Fatalf("Total.Agents = %d, want 1 (readme-only must be skipped)", section.Total.Agents)
	}
	alpha, ok := section.PerAgent["alpha"]
	if !ok {
		t.Fatalf("PerAgent missing alpha: %#v", section.PerAgent)
	}
	if alpha.States != 4 {
		t.Errorf("alpha.States = %d, want 4 (3 from machine.yaml + 1 from request-machine.yaml)", alpha.States)
	}
	if alpha.Transitions != 3 {
		t.Errorf("alpha.Transitions = %d, want 3 (2 + 1 across machine files)", alpha.Transitions)
	}
	if alpha.Tools != 2 {
		t.Errorf("alpha.Tools = %d, want 2 (declarations.yaml and profile.yaml must not count)", alpha.Tools)
	}
	if alpha.YAML.Files != 6 {
		t.Errorf("alpha.YAML.Files = %d, want 6 (all YAML files, recursively)", alpha.YAML.Files)
	}
	if alpha.YAML.Lines == 0 {
		t.Error("alpha.YAML.Lines = 0, want a positive line count")
	}
	if section.Total.States != alpha.States || section.Total.Tools != alpha.Tools {
		t.Errorf("Total = %+v, want it to equal the single agent %+v", section.Total, alpha)
	}
}

// TestScanAgentsMissingDir proves a module without an agents/ directory
// reports an empty section rather than an error.
func TestScanAgentsMissingDir(t *testing.T) {
	t.Parallel()
	section, err := scanAgents(filepath.Join(t.TempDir(), "agents"), profileCountLines)
	if err != nil {
		t.Fatalf("scanAgents returned error: %v", err)
	}
	if section.Total.Agents != 0 || len(section.PerAgent) != 0 {
		t.Fatalf("section = %+v, want empty", section)
	}
}

// TestScanAgentsBadYAML proves a malformed machine file surfaces as an error
// instead of silently zeroing the counts.
func TestScanAgentsBadYAML(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	agentsDir := filepath.Join(root, "agents")
	writeAgentFixture(t, filepath.Join(agentsDir, "broken", "machine.yaml"), "states: [unclosed\n")

	if _, err := scanAgents(agentsDir, profileCountLines); err == nil {
		t.Fatal("scanAgents = nil error, want parse failure for malformed machine.yaml")
	}
}
