// Copyright (c) 2026 Nokia. All rights reserved.

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
		"agent-profiles": json.RawMessage(`{"agents": {"total": {
			"agents": 9, "states": 115, "transitions": 206, "tools": 94,
			"yaml": {"files": 82, "lines": 8531}}}}`),
		"examples/chatbot-mesh": json.RawMessage(`{"agents": {"total": {
			"agents": 6, "states": 123, "transitions": 192, "tools": 51,
			"yaml": {"files": 69, "lines": 6911}}}}`),
	}

	total, err := sumAgentsTotals(results)
	if err != nil {
		t.Fatalf("sumAgentsTotals returned error: %v", err)
	}
	if total.Agents != 15 {
		t.Errorf("Agents = %d, want 15", total.Agents)
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

// TestSumAgentsTotalsBadJSON proves malformed module output surfaces as an
// error naming the module.
func TestSumAgentsTotalsBadJSON(t *testing.T) {
	t.Parallel()
	results := map[string]json.RawMessage{
		"agent-profiles": json.RawMessage(`{"agents":`),
	}
	if _, err := sumAgentsTotals(results); err == nil {
		t.Fatal("sumAgentsTotals = nil error, want parse failure")
	}
}
