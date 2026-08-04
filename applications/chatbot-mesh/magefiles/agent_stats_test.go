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

	section, err := scanAgents(agentsDir, meshCountLines)
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

func TestScanAgentOwnershipSeparatesCompositionWrapper(t *testing.T) {
	t.Parallel()
	agentsDir := filepath.Join(t.TempDir(), "agents")
	writeAgentFixture(t, filepath.Join(agentsDir, "alpha", "machine.yaml"), fixtureMachine)
	writeAgentFixture(t, filepath.Join(agentsDir, "alpha", "tools.yaml"), fixtureTools)
	writeAgentFixture(t, filepath.Join(agentsDir, "alpha", "profile.yaml"), fixtureProfile)
	writeAgentFixture(t, filepath.Join(agentsDir, "corpus-ingest", "profile.yaml"), `name: corpus-ingest
machine: ../knowledge-manager/corpus-ingest/machine.yaml
tools:
  - ../knowledge-manager/corpus-ingest/tools.yaml
`)
	writeAgentFixture(t, filepath.Join(agentsDir, "corpus-ingest", "corpus-rest.yaml"),
		"clients: {}\n")

	stats, err := scanAgentOwnership(agentsDir, meshCountLines)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Agents.Total.Agents != 1 ||
		len(stats.Agents.PerAgent) != 1 ||
		stats.Agents.PerAgent["alpha"].States != 3 {
		t.Fatalf("local agents = %+v, want only alpha implementation", stats.Agents)
	}
	if _, exists := stats.Agents.PerAgent["corpus-ingest"]; exists {
		t.Fatal("composition wrapper appears in implementation agents")
	}
	wrapper, exists := stats.Composition.PerWrapper["corpus-ingest"]
	if !exists {
		t.Fatalf("composition = %+v, missing corpus-ingest", stats.Composition)
	}
	if wrapper.Ownership != "composition" ||
		wrapper.CanonicalSource != "applications/catalog" ||
		wrapper.CanonicalProgram != "agents/knowledge-manager/corpus-ingest" {
		t.Fatalf("wrapper = %+v", wrapper)
	}
	if stats.Composition.Total.Wrappers != 1 ||
		stats.Composition.Total.CanonicalReferences != 1 ||
		stats.Composition.Total.YAML != wrapper.YAML {
		t.Fatalf("composition total = %+v, wrapper = %+v",
			stats.Composition.Total, wrapper)
	}
}

func TestRepositoryMeshOwnershipClassifiesCorpusIngestAsComposition(t *testing.T) {
	stats, err := scanAgentOwnership(filepath.Join("..", "agents"), meshCountLines)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := stats.Agents.PerAgent["corpus-ingest"]; exists {
		t.Fatal("repository corpus-ingest wrapper counted as local implementation")
	}
	if stats.Composition.PerWrapper["corpus-ingest"].CanonicalProgram !=
		"agents/knowledge-manager/corpus-ingest" {
		t.Fatalf("repository composition = %+v", stats.Composition)
	}
	if _, exists := stats.Agents.PerAgent["applier"]; exists {
		t.Fatal("repository applier wrapper counted as local implementation")
	}
	if stats.Composition.PerWrapper["applier"].CanonicalProgram != "agents/applier" {
		t.Fatalf("repository applier composition = %+v", stats.Composition)
	}
	if stats.Agents.Total.Agents != len(stats.Agents.PerAgent) {
		t.Fatalf("agent total %d != per-agent count %d",
			stats.Agents.Total.Agents, len(stats.Agents.PerAgent))
	}
}

// TestScanAgentsMissingDir proves a module without an agents/ directory
// reports an empty section rather than an error.
func TestScanAgentsMissingDir(t *testing.T) {
	t.Parallel()
	section, err := scanAgents(filepath.Join(t.TempDir(), "agents"), meshCountLines)
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

	if _, err := scanAgents(agentsDir, meshCountLines); err == nil {
		t.Fatal("scanAgents = nil error, want parse failure for malformed machine.yaml")
	}
}
