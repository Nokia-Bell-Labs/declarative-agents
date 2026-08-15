// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"encoding/json"
	"testing"
)

// TestSumAgentsTotals proves the repo-wide agents total sums the per-module
// "agents.total" sections and ignores modules that report no agents.
func TestSumAgentsTotals(t *testing.T) {
	t.Parallel()
	results := map[string]json.RawMessage{
		"agent-core": json.RawMessage(`{"go": {"src_lines": 10}}`),
		"applications/catalog": json.RawMessage(`{"agents": {"total": {
			"agents": 9, "states": 115, "transitions": 206, "tools": 94,
			"yaml": {"files": 82, "lines": 8531}}}}`),
		"applications/chatbot-mesh": json.RawMessage(`{"agents": {"total": {
			"agents": 5, "states": 123, "transitions": 192, "tools": 51,
			"yaml": {"files": 69, "lines": 6911}}},
			"composition": {"total": {
				"wrappers": 2, "canonical_references": 2,
				"yaml": {"files": 2, "lines": 271}},
				"per_wrapper": {"corpus-ingest": {
					"ownership": "composition-wrapper",
					"canonical_source": "applications/catalog",
					"canonical_program": "agents/knowledge-manager/corpus-ingest",
					"yaml": {"files": 2, "lines": 271}}}},
			"application": {
				"ownership": "agent-owning", "agents_contributed": 5,
				"composition_wrappers": 2}}`),
		"applications/agent-architecture": json.RawMessage(`{
			"application": {
				"ownership": "composition-only",
				"agents_contributed": 0,
				"canonical_references": 1,
				"canonical_profile": "applications/catalog/agents/knowledge-manager/documentation-curator/profile.yaml"
			}}`),
	}

	var demo map[string]json.RawMessage
	if err := json.Unmarshal(results["applications/agent-architecture"], &demo); err != nil {
		t.Fatal(err)
	}
	if _, exists := demo["application"]; !exists {
		t.Fatal("Agent Architecture stats must contain an application composition section")
	}
	if _, exists := demo["agents"]; exists {
		t.Fatal("Agent Architecture stats must report composition without an agents section")
	}
	total, err := sumAgentsTotals(results)
	if err != nil {
		t.Fatalf("sumAgentsTotals returned error: %v", err)
	}
	if total.Agents != 14 {
		t.Errorf("Agents = %d, want 14", total.Agents)
	}
	if total.States != 238 {
		t.Errorf("States = %d, want 238", total.States)
	}
	if total.Transitions != 398 {
		t.Errorf("Transitions = %d, want 398", total.Transitions)
	}
	if total.Tools != 145 {
		t.Errorf("Tools = %d, want 145", total.Tools)
	}
	if total.YAML.Files != 151 || total.YAML.Lines != 15442 {
		t.Errorf("YAML = %+v, want {Files: 151, Lines: 15442}", total.YAML)
	}
}

func TestSumAgentsTotalsValidatesPerAgentConsistency(t *testing.T) {
	t.Parallel()
	results := map[string]json.RawMessage{
		"mixed-example": json.RawMessage(`{"agents": {
			"total": {"agents": 2, "states": 3, "transitions": 2, "tools": 1,
				"yaml": {"files": 3, "lines": 30}},
			"per_agent": {
				"local-a": {"states": 2, "transitions": 1, "tools": 1,
					"yaml": {"files": 2, "lines": 20}},
				"local-b": {"states": 1, "transitions": 1, "tools": 0,
					"yaml": {"files": 1, "lines": 10}}
			}},
			"composition": {"total": {"wrappers": 1}}}`),
	}
	if _, err := sumAgentsTotals(results); err != nil {
		t.Fatalf("consistent mixed ownership rejected: %v", err)
	}

	results["mixed-example"] = json.RawMessage(`{"agents": {
		"total": {"agents": 3, "states": 3, "transitions": 2, "tools": 1,
			"yaml": {"files": 3, "lines": 30}},
		"per_agent": {
			"local-a": {"states": 2, "transitions": 1, "tools": 1,
				"yaml": {"files": 2, "lines": 20}},
			"local-b": {"states": 1, "transitions": 1, "tools": 0,
				"yaml": {"files": 1, "lines": 10}}
		}}}`)
	if _, err := sumAgentsTotals(results); err == nil {
		t.Fatal("inconsistent agents.total accepted")
	}
}

// TestSumAgentsTotalsBadJSON proves malformed module output surfaces as an
// error naming the module.
func TestSumAgentsTotalsBadJSON(t *testing.T) {
	t.Parallel()
	results := map[string]json.RawMessage{
		"applications/catalog": json.RawMessage(`{"agents":`),
	}
	if _, err := sumAgentsTotals(results); err == nil {
		t.Fatal("sumAgentsTotals = nil error, want parse failure")
	}
}
